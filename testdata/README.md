# testdata

Solo lo estrictamente necesario para demostrar comportamientos concretos.
Para probar cargas reales usa tus propios archivos.

| Archivo     | Para que sirve |
|-------------|----------------|
| `smoke.sh`  | Recorre el flujo completo de extremo a extremo con curl. Requiere `jq`. |
| `eicar.txt` | Archivo de prueba EICAR. No es un virus: es un texto que todos los antivirus reconocen como si lo fuera, creado justamente para probar la deteccion. Subelo como `kind=file` y el asset debe quedar en estado `infected`, con el objeto borrado del almacenamiento y una linea `asset.infected` en la auditoria. |
