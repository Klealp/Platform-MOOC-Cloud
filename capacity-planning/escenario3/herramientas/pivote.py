"""Une los CSV que exporta series.py en una linea de tiempo por minuto.

Uso: python pivote.py DIR GRUPO   (GRUPO = prefijo de las VMs: mooc-api o mooc-worker)

Columnas: tamano de los grupos, cola (backlog = pendientes + activos, y su
desglose), CPU por VM del grupo indicado (ultimos 4 caracteres del nombre),
CPU y conexiones de Cloud SQL y peticiones/s del backend de la API en el
balanceador.
"""
import csv, os, sys
from collections import defaultdict

d, grupo = sys.argv[1], sys.argv[2]
t = defaultdict(dict)


def load(name):
    p = os.path.join(d, name + ".csv")
    if not os.path.exists(p):  # exportaciones anteriores sin esa serie
        return []
    with open(p) as f:
        return list(csv.DictReader(f))


def minuto(r):
    return r["tiempo_utc"][11:16]


for r in load("instance_group_size"):
    t[minuto(r)]["n_" + r["serie"]] = int(float(r["valor"]))
for serie, clave in (("queue_backlog", "cola"), ("queue_pending", "pend"), ("queue_active", "act"),
                     ("queue_retry", "reint"), ("queue_oldest_pending_seconds", "antig")):
    for r in load(serie):
        t[minuto(r)][clave] = int(float(r["valor"]))
for r in load("cloudsql_cpu_utilization"):
    t[minuto(r)]["sql"] = float(r["valor"]) * 100
for r in load("cloudsql_num_backends"):
    t[minuto(r)]["con"] = int(float(r["valor"]))
for r in load("lb_request_count"):
    t[minuto(r)]["rps"] = float(r["valor"])
for r in load("vm_cpu_utilization"):
    if r["serie"].startswith(grupo):
        t[minuto(r)].setdefault("cpu", {})[r["serie"][-4:]] = round(float(r["valor"]) * 100)

print("min | api | wrk | backlog | pend | act | reint | antig_s | cpu " + grupo + " | sql% | con | lb rps")
for m in sorted(t):
    x = t[m]
    cpu = " ".join(f"{k}:{v}" for k, v in sorted(x.get("cpu", {}).items()))
    cols = [x.get(k, "-") for k in ("n_mooc-api", "n_mooc-worker", "cola", "pend", "act", "reint", "antig")]
    print(f"{m} | " + " | ".join(str(c) for c in cols) + f" | {cpu} | {x.get('sql', 0):.0f} | {x.get('con', '-')} | {x.get('rps', 0):.1f}")
