package app

import (
	"embed"
	"io/fs"
)

// La pantalla de la demo (DEMO-3, #145) viaja dentro del binario: el día de
// la defensa no depende de internet ni de archivos sueltos.
//
//go:embed demoweb
var demoWeb embed.FS

// demoAssets devuelve la interfaz que sirve demo-server en "/".
func demoAssets() fs.FS {
	assets, err := fs.Sub(demoWeb, "demoweb")
	if err != nil {
		panic(err)
	}
	return assets
}
