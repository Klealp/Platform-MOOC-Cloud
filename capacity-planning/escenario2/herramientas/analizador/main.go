// Command analizador convierte los resultados crudos de una corrida del
// Escenario 2 en tablas (Markdown) y graficas (SVG) para el informe.
//
// Una corrida:
//
//	go run ./capacity-planning/escenario2/herramientas/analizador \
//	    -corrida capacity-planning/escenario2/resultados/20261001-1030_n1_nube
//
//	-> <corrida>/analisis.md, <corrida>/cargas.csv, <corrida>/graficas/*.svg
//
// Comparar niveles (la tabla de capacidad del informe):
//
//	go run ./capacity-planning/escenario2/herramientas/analizador \
//	    -comparar resultados/..._base_nube,resultados/..._n1_nube,resultados/..._n2_nube \
//	    -salida capacity-planning/escenario2/resultados/comparativa-nube
//
// Solo usa la biblioteca estandar de Go.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	corrida := flag.String("corrida", "", "carpeta de UNA corrida (resultados/<fecha>_<nivel>_<modo>)")
	comparar := flag.String("comparar", "", "carpetas de varias corridas separadas por coma, en orden de nivel")
	salida := flag.String("salida", "", "carpeta de salida para -comparar")
	flag.Parse()

	switch {
	case *corrida != "":
		c, err := cargarCorrida(*corrida)
		if err != nil {
			log.Fatal(err)
		}
		ind := calcular(c)
		if err := escribirAnalisis(c, ind); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Analisis escrito en %s\n", filepath.Join(*corrida, "analisis.md"))
		if len(c.Faltantes) > 0 {
			fmt.Printf("Aviso: faltan datos: %s\n", strings.Join(c.Faltantes, ", "))
		}
	case *comparar != "":
		if *salida == "" {
			log.Fatal("-comparar requiere -salida")
		}
		var inds []Indicadores
		for _, d := range strings.Split(*comparar, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			c, err := cargarCorrida(d)
			if err != nil {
				log.Fatal(err)
			}
			inds = append(inds, calcular(c))
		}
		if err := os.MkdirAll(*salida, 0o755); err != nil {
			log.Fatal(err)
		}
		if err := escribirComparativa(*salida, inds); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Comparativa escrita en %s\n", filepath.Join(*salida, "comparativa.md"))
	default:
		flag.Usage()
		os.Exit(2)
	}
}
