package app

import "io/fs"

// demoAssets devuelve la interfaz que sirve demo-server. La pantalla se agrega
// en DEMO-3 (#145); hasta entonces el servidor expone sólo la API.
func demoAssets() fs.FS {
	return nil
}
