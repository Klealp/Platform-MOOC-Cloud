"""Exporta series de Cloud Monitoring a CSV para los escenarios 3A y 3B.

Uso: python series.py TOKEN SALIDA_DIR INICIO FIN PREFIJO
"""
import csv, json, sys, urllib.parse, urllib.request, os

TOKEN, OUT, START, END, PREF = sys.argv[1:6]
PROJECT = "dsc-uniandes-20262"
os.makedirs(OUT, exist_ok=True)

with open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "instancias.csv")) as f:
    NOMBRES = {r["instance_id"]: r["nombre"] for r in csv.DictReader(f)}

SERIES = {
    "instance_group_size": ('metric.type="compute.googleapis.com/instance_group/size"', "ALIGN_MAX", None),
    "queue_backlog": ('metric.type="custom.googleapis.com/mooc/queue_backlog"', "ALIGN_MAX", None),
    # Desglose que publica cmd/queue-metrics junto al backlog
    "queue_pending": ('metric.type="custom.googleapis.com/mooc/queue_pending"', "ALIGN_MAX", None),
    "queue_active": ('metric.type="custom.googleapis.com/mooc/queue_active"', "ALIGN_MAX", None),
    "queue_retry": ('metric.type="custom.googleapis.com/mooc/queue_retry"', "ALIGN_MAX", None),
    "queue_oldest_pending_seconds": ('metric.type="custom.googleapis.com/mooc/queue_oldest_pending_seconds"', "ALIGN_MAX", None),
    "vm_cpu_utilization": ('metric.type="compute.googleapis.com/instance/cpu/utilization"', "ALIGN_MEAN", None),
    # Memoria por VM: solo la publica el Ops Agent que instala startup.sh
    "vm_memory_percent_used": ('metric.type="agent.googleapis.com/memory/percent_used" AND metric.label.state="used"', "ALIGN_MEAN", None),
    "cloudsql_cpu_utilization": ('metric.type="cloudsql.googleapis.com/database/cpu/utilization"', "ALIGN_MEAN", None),
    "cloudsql_num_backends": ('metric.type="cloudsql.googleapis.com/database/postgresql/num_backends"', "ALIGN_MAX", "REDUCE_SUM"),
    "lb_request_count": ('metric.type="loadbalancing.googleapis.com/https/backend_request_count" AND resource.label.backend_target_name="mooc-api-be"', "ALIGN_RATE", "REDUCE_SUM"),
}


def fetch(flt, aligner, reducer):
    q = {
        "filter": flt,
        "interval.startTime": START,
        "interval.endTime": END,
        "aggregation.alignmentPeriod": "60s",
        "aggregation.perSeriesAligner": aligner,
    }
    if reducer:
        q["aggregation.crossSeriesReducer"] = reducer
    url = f"https://monitoring.googleapis.com/v3/projects/{PROJECT}/timeSeries?" + urllib.parse.urlencode(q)
    out, token = [], None
    while True:
        u = url + (f"&pageToken={token}" if token else "")
        req = urllib.request.Request(u, headers={"Authorization": f"Bearer {TOKEN}"})
        d = json.load(urllib.request.urlopen(req))
        out += d.get("timeSeries", [])
        token = d.get("nextPageToken")
        if not token:
            return out


def label(ts):
    m, r = ts["metric"].get("labels", {}), ts["resource"].get("labels", {})
    for k in ("instance_group_name", "instance_name", "database_id", "queue"):
        if k in m:
            return m[k]
        if k in r:
            return r[k]
    if "instance_id" in r:
        # Las metricas del Ops Agent no traen el nombre de la VM, solo su id.
        # Las VMs retiradas por el autoescalador ya no existen, asi que el
        # nombre sale de instancias.csv (armado con los logs de gcplogs).
        return NOMBRES.get(r["instance_id"], r["instance_id"])
    return "total"


for name, (flt, al, red) in SERIES.items():
    try:
        series = fetch(flt, al, red)
    except Exception as e:  # una metrica sin datos no detiene el resto
        print(name, "ERROR", e)
        continue
    rows = []
    for ts in series:
        lab = label(ts)
        if name.startswith("vm_") and not lab.startswith(("mooc-api", "mooc-worker", "infra")):
            continue
        for p in ts["points"]:
            v = p["value"]
            val = v.get("doubleValue", v.get("int64Value"))
            rows.append((p["interval"]["endTime"][:19] + "Z", lab, val))
    rows.sort()
    with open(os.path.join(OUT, f"{PREF}{name}.csv"), "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["tiempo_utc", "serie", "valor"])
        w.writerows(rows)
    print(name, len(rows), "puntos", sorted({r[1] for r in rows}))
