package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/Nach0Zar/tesis-serra-zarlenga-fabric/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool        *pgxpool.Pool
	credentials Credentials
	now         func() time.Time
	newID       func() (string, error)
}

func NewStore(pool *pgxpool.Pool, credentials Credentials) *Store {
	return &Store{pool: pool, credentials: credentials, now: time.Now, newID: randomID}
}

func (s *Store) Authenticate(key string) (Credential, error) {
	credential, ok := s.credentials.Resolve(key)
	if !ok {
		return Credential{}, NewError(UnauthorizedRole, "la credencial X-Org-Key no habilita esta operacion")
	}
	return credential, nil
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func formatTimestamp(value time.Time) string { return value.UTC().Format(time.RFC3339) }

func (s *Store) begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, internal(err, "no se pudo iniciar la transaccion")
	}
	return tx, nil
}

func commit(ctx context.Context, tx pgx.Tx) error {
	if err := tx.Commit(ctx); err != nil {
		return internal(err, "no se pudo confirmar la transaccion")
	}
	return nil
}

func resolveInvoker(ctx context.Context, tx pgx.Tx, credential Credential) (Invoker, error) {
	org, found, err := readOrganization(ctx, tx, credential.MSPID)
	if err != nil {
		return Invoker{}, err
	}
	if !found {
		return Invoker{}, NewError(OrgNotRegistered, "la organizacion %s no tiene entrada en el registro", credential.MSPID).WithDetails(map[string]any{"mspId": credential.MSPID})
	}
	if !org.Active {
		return Invoker{}, NewError(OrgInactive, "la organizacion %s esta registrada pero no habilitada", credential.MSPID).WithDetails(map[string]any{"mspId": credential.MSPID})
	}
	return Invoker{MSPID: credential.MSPID, Org: org, Role: credential.Role}, nil
}

func resolveRegulator(ctx context.Context, tx pgx.Tx, credential Credential) (Invoker, error) {
	invoker, err := resolveInvoker(ctx, tx, credential)
	if err != nil {
		if code, ok := ErrorCode(err); ok && (code == OrgNotRegistered || code == OrgInactive) {
			return Invoker{}, regulatoryOnly()
		}
		return Invoker{}, err
	}
	if invoker.Org.AgentType != domain.AgentRegulator || invoker.Role != RoleRegulatoryAdmin {
		return Invoker{}, regulatoryOnly()
	}
	return invoker, nil
}

func regulatoryOnly() error {
	return NewError(RegulatoryOnly, "la operacion exige una organizacion REGULATOR activa y rol regulatory-admin")
}

func requireRole(invoker Invoker, roles ...string) error {
	for _, role := range roles {
		if invoker.Role == role {
			return nil
		}
	}
	return NewError(UnauthorizedRole, "el rol %q no habilita esta operacion", invoker.Role).WithDetails(map[string]any{"snt.role": invoker.Role, "rolesHabilitados": roles})
}

func requireAgentType(invoker Invoker, types ...domain.AgentType) error {
	for _, agentType := range types {
		if invoker.Org.AgentType == agentType {
			return nil
		}
	}
	return NewError(UnauthorizedAgentType, "el agentType %s no puede ejecutar esta operacion", invoker.Org.AgentType).WithDetails(map[string]any{"agentType": string(invoker.Org.AgentType)})
}

func readOrganization(ctx context.Context, tx pgx.Tx, mspID string) (Organization, bool, error) {
	var org Organization
	err := tx.QueryRow(ctx, `SELECT msp_id, id, id_type, agent_type, active FROM public.organizations WHERE msp_id=$1`, mspID).
		Scan(&org.MSPID, &org.ID, &org.IDType, &org.AgentType, &org.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, false, nil
	}
	if err != nil {
		return Organization{}, false, internal(err, "no se pudo leer el registro organizacion-establecimiento")
	}
	return org, true, nil
}

func lookupOrganizationByCanonicalID(ctx context.Context, tx pgx.Tx, canonicalID string) (Organization, error) {
	idType, id, err := parseCanonicalID(canonicalID)
	if err != nil {
		return Organization{}, err
	}
	var org Organization
	err = tx.QueryRow(ctx, `SELECT msp_id, id, id_type, agent_type, active FROM public.organizations WHERE id_type=$1 AND id=$2`, idType, id).
		Scan(&org.MSPID, &org.ID, &org.IDType, &org.AgentType, &org.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, NewError(OrgNotRegistered, "el identificador %s no tiene entrada en el registro", canonicalID).WithDetails(map[string]any{"id": canonicalID})
	}
	if err != nil {
		return Organization{}, internal(err, "no se pudo resolver el identificador canonico")
	}
	return org, nil
}

func resolveDestination(ctx context.Context, tx pgx.Tx, declared string) (Organization, error) {
	var org Organization
	var err error
	if stringsContainsColon(declared) {
		org, err = lookupOrganizationByCanonicalID(ctx, tx, declared)
	} else {
		var found bool
		org, found, err = readOrganization(ctx, tx, declared)
		if err == nil && !found {
			err = NewError(OrgNotRegistered, "el destino %q no tiene entrada en el registro", declared).WithDetails(map[string]any{"destino": declared})
		}
	}
	if err != nil {
		return Organization{}, err
	}
	if !org.Active {
		return Organization{}, NewError(OrgInactive, "el destino %q esta registrado pero no habilitado", declared).WithDetails(map[string]any{"destino": declared})
	}
	custodial, err := domain.IsCustodialAgentType(org.AgentType)
	if err != nil {
		return Organization{}, internal(err, "no se pudo consultar el catalogo de agentType")
	}
	if !custodial {
		return Organization{}, NewError(InvalidDestination, "el agentType %s no puede ser destino de una transferencia", org.AgentType)
	}
	return org, nil
}

func stringsContainsColon(value string) bool {
	for _, character := range value {
		if character == ':' {
			return true
		}
	}
	return false
}

func requireTransition(from domain.State, event domain.Event, actor domain.Actor) (domain.Transition, error) {
	transition, ok := domain.LookupTransition(from, event)
	if !ok {
		return domain.Transition{}, NewError(InvalidStateTransition, "la maquina de estados no declara el evento %s desde el estado %s", event, from).WithDetails(map[string]any{"estado": string(from), "evento": string(event)})
	}
	if !transition.AllowsActor(actor) {
		return domain.Transition{}, NewError(InvalidStateTransition, "la transicion %s no habilita al actor %s", transition.ID, actor)
	}
	return transition, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
