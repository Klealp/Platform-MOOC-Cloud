// Package openapi incrusta el contrato OpenAPI dentro del binario de la API.
//
// Se embebe con go:embed en vez de leerlo de disco por dos razones: el binario
// sigue siendo autocontenido (la imagen final solo copia el ejecutable, no el
// arbol de fuentes) y la ruta /openapi.yaml no depende del directorio de
// trabajo del proceso. El archivo openapi.yaml de esta carpeta es la UNICA
// fuente de verdad del contrato; la API lo sirve tal cual en /openapi.yaml y lo
// pinta con Swagger UI en /docs.
package openapi

import _ "embed"

//go:embed openapi.yaml
var Spec []byte
