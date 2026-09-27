// =====================================================================
// Escenario 2 - Carga, procesamiento y consumo multimedia (k6)
//
// Dos grupos de usuarios virtuales corren AL MISMO TIEMPO:
//
//  profesores   (tasa constante de llegada: N cargas por minuto)
//     1. POST /uploads                      -> la API autoriza y firma las URLs
//     2. PUT de cada parte (8 MiB) DIRECTO al almacenamiento de objetos
//     3. POST /uploads/{id}/complete        -> confirmar la carga completa
//     4. POST /units/{id}/resources         -> crear el recurso en su curso
//     5. GET  /assets/{id} cada 3 s         -> hasta ready / failed / infected
//     6. Validacion funcional: duracion y numero de rendiciones del master
//
//  espectadores (usuarios concurrentes fijos)
//     1. GET /assets/{id}/playlist          -> master .m3u8 (API)
//     2. GET playlist de UNA rendicion      -> .m3u8 con segmentos firmados (API)
//     3. GET de cada segmento .ts DIRECTO al almacenamiento, al ritmo de
//        reproduccion: pre-carga 3 segmentos y despues mantiene 3 de ventaja
//        sobre la posicion de reproduccion (cadencia = EXTINF).
//
// Todo lo que cambia entre niveles vive en ../niveles.json; los datos
// (tokens, unidades, contenido HLS) en ../datos/seed.json (preparar_datos.sh)
// y los archivos en ../media/ (generar_perfiles.sh).
//
// Uso tipico: ver correr_nivel.sh. Directo:
//   NIVEL=humo k6 run capacity-planning/escenario2/k6/escenario2.js
// =====================================================================
import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { Trend, Counter, Rate } from 'k6/metrics';
import { open as abrirArchivo, SeekMode } from 'k6/experimental/fs';

// ------------------------- Configuracion -------------------------
const NIVELES = JSON.parse(open(__ENV.NIVELES || '../niveles.json'));
const NIVEL_NOMBRE = __ENV.NIVEL || 'humo';
const NIVEL = NIVELES.niveles[NIVEL_NOMBRE];
if (!NIVEL) throw new Error(`Nivel desconocido: ${NIVEL_NOMBRE}`);
const C = NIVELES.comun;

// Permite ajustar un nivel sin editar el JSON (se registra en metadata.json).
const CARGAS_POR_MIN = Number(__ENV.CARGAS_POR_MIN || NIVEL.cargas_por_min);
const ESPECTADORES = Number(__ENV.ESPECTADORES || NIVEL.espectadores);
const DURACION = __ENV.DURACION || NIVEL.duracion;
const CALENTAMIENTO = __ENV.CALENTAMIENTO || NIVEL.calentamiento;
const DRENAJE_MAX = __ENV.DRENAJE_MAX || NIVEL.drenaje_max;

const SEED = JSON.parse(open(__ENV.SEED || '../datos/seed.json'));
const MEDIA_DIR = __ENV.MEDIA_DIR || '../media/';
const PERFILES = JSON.parse(open(`${MEDIA_DIR}perfiles.json`)).perfiles;
const PERFIL = Object.fromEntries(PERFILES.map((p) => [p.id, p]));
const API_URL = (__ENV.API_URL || SEED.api_url).replace(/\/$/, '');
const API = `${API_URL}/api/v1`;

const POLL_S = Number(C.poll_estado_s);
const TIMEOUT_READY_MS = Number(C.timeout_ready_s) * 1000;
const PATRON = C.patron_perfiles;

// Archivos multimedia: se abren UNA vez por usuario virtual con el modulo fs
// de k6, que comparte el contenido en memoria entre VUs (open(...,'b')
// copiaria 50 MB por cada VU). Cada parte se lee con seek + read.
const ARCHIVOS = {};
(async function () {
  for (const p of PERFILES) ARCHIVOS[p.id] = await abrirArchivo(`${MEDIA_DIR}${p.archivo}`);
})();

// ------------------------- Escenarios -------------------------
function segundos(d) {
  const m = String(d).match(/^(\d+(?:\.\d+)?)(ms|s|m|h)$/);
  if (!m) throw new Error(`duracion invalida: ${d}`);
  return Number(m[1]) * { ms: 0.001, s: 1, m: 60, h: 3600 }[m[2]];
}
const CAL_S = segundos(CALENTAMIENTO);
const DUR_S = segundos(DURACION);
// Cuantas cargas pueden estar vivas a la vez en el peor caso: todas las del
// nivel (en saturacion ninguna termina antes del final).
const MAX_VUS_PROF = Math.max(2, Math.ceil(CARGAS_POR_MIN * (DUR_S / 60)) + 2);

const escenarios = {};
if (CARGAS_POR_MIN > 0) {
  escenarios.profesores = {
    executor: 'constant-arrival-rate',
    exec: 'profesor',
    rate: 1,
    // 1 carga cada (60 / cargas_por_min) segundos: sirve tambien para tasas < 1/min.
    timeUnit: `${Math.round(60000 / CARGAS_POR_MIN)}ms`,
    duration: DURACION,
    startTime: CALENTAMIENTO, // las cargas empiezan cuando los espectadores ya estan en regimen
    preAllocatedVUs: MAX_VUS_PROF,
    maxVUs: MAX_VUS_PROF,
    // Las cargas ya aceptadas se siguen observando hasta ready (drenaje de la cola).
    gracefulStop: DRENAJE_MAX,
    tags: { actor: 'profesor' },
  };
}
if (ESPECTADORES > 0) {
  escenarios.espectadores = {
    executor: 'ramping-vus',
    exec: 'espectador',
    startVUs: 0,
    stages: [
      { duration: CALENTAMIENTO, target: ESPECTADORES }, // calentamiento: rampa
      { duration: DURACION, target: ESPECTADORES }, // medicion: constante
    ],
    gracefulRampDown: '10s',
    gracefulStop: '20s',
    tags: { actor: 'espectador' },
  };
}

// Nombres de las operaciones (tag "name"): el analizador agrupa por ellos.
const OP = {
  init: 'POST /uploads (autorizar y firmar)',
  parte: 'PUT parte (almacenamiento)',
  complete: 'POST /uploads/:id/complete (confirmar)',
  recurso: 'POST /units/:id/resources',
  estado: 'GET /assets/:id (consultar estado)',
  masterProf: 'GET playlist maestro (validacion profesor)',
  master: 'GET playlist maestro',
  variante: 'GET playlist variante',
  segmento: 'GET segmento (almacenamiento)',
};

export const options = {
  scenarios: escenarios,
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max', 'count'],
  // Criterios de exito declarados ANTES de las pruebas (ver README). Se
  // evaluan solo sobre la fase de medicion (sin calentamiento). Tener un
  // umbral por operacion hace que k6 reporte cada una por separado.
  thresholds: {
    'http_req_failed{tipo:control,fase:medicion}': ['rate<0.01'],
    'http_req_failed{tipo:transferencia,fase:medicion}': ['rate<0.01'],
    'http_req_failed{tipo:consumo,fase:medicion}': ['rate<0.01'],
    'http_req_failed{tipo:consumo_api,fase:medicion}': ['rate<0.01'],
    [`http_req_duration{name:${OP.init}}`]: ['p(95)<1000'],
    [`http_req_duration{name:${OP.complete}}`]: ['p(95)<1000'],
    [`http_req_duration{name:${OP.recurso}}`]: ['p(95)<1000'],
    [`http_req_duration{name:${OP.estado}}`]: ['p(95)<500'],
    [`http_req_duration{name:${OP.parte}}`]: ['p(95)<30000'],
    'e2_manifiesto_ms{fase:medicion}': ['p(95)<1000'],
    'e2_segmento_ms{fase:medicion}': ['p(95)<2000'],
    'e2_riesgo_corte{fase:medicion}': ['rate<0.01'],
    e2_carga_exitosa: ['rate>0.99'],
    e2_api_firma_ms: ['p(95)<1000'],
  },
  // Solo se miden los tiempos: no hace falta guardar los cuerpos de los
  // segmentos (cada peticion que SI necesita el cuerpo lo pide explicito).
  discardResponseBodies: true,
};

// ------------------------- Metricas propias -------------------------
// Profesores (plano de control vs. transferencia directa)
const mFirma = new Trend('e2_api_firma_ms', true);
const mTransf = new Trend('e2_transferencia_ms', true);
const mTransfMbps = new Trend('e2_transferencia_mbps');
const mConfirma = new Trend('e2_confirmacion_ms', true);
const mHastaProc = new Trend('e2_espera_hasta_procesar_ms', true); // complete -> visto "processing" (cola scan + escaneo)
const mReady = new Trend('e2_carga_a_ready_ms', true); // complete -> visto "ready"
const mE2E = new Trend('e2_extremo_a_extremo_ms', true); // inicio -> ready
const mExito = new Rate('e2_carga_exitosa');
const mFallos = new Counter('e2_cargas_fallidas');
// Espectadores
const mManifiesto = new Trend('e2_manifiesto_ms', true);
const mSegmento = new Trend('e2_segmento_ms', true);
const mSegMbps = new Trend('e2_segmento_mbps');
const mCorte = new Rate('e2_riesgo_corte'); // el segmento llego despues de que "deberia" reproducirse
const mRechazos = new Counter('e2_rechazos_429');

function fase() {
  const t = (Date.now() - exec.scenario.startTime) / 1000;
  // Para los espectadores el reloj del escenario empieza con la rampa.
  if (exec.scenario.name === 'espectadores' && t < CAL_S) return 'calentamiento';
  return 'medicion';
}

function registrar429(res) {
  if (res.status === 429) mRechazos.add(1);
}

// =====================================================================
// PROFESOR
// =====================================================================
export async function profesor() {
  const it = exec.scenario.iterationInTest;
  const pid = PATRON[it % PATRON.length];
  const p = PERFIL[pid];
  const prof = SEED.profesores[it % SEED.profesores.length];
  const auth = { Authorization: `Bearer ${prof.token}`, 'Content-Type': 'application/json' };
  const tags = (op, tipo) => ({ name: op, tipo, perfil: pid, fase: 'medicion' });
  const reg = { iter: it, perfil: pid, profesor: prof.email, tamano_bytes: p.tamano_bytes, nivel: NIVEL_NOMBRE };

  const fallar = (motivo, res) => {
    reg.estado_final = 'fallo';
    reg.motivo = motivo;
    if (res) reg.http = res.status;
    mExito.add(false, { perfil: pid });
    mFallos.add(1, { perfil: pid, motivo });
    console.log('E2_CARGA ' + JSON.stringify(reg));
  };

  // 1. Autorizar la carga y obtener las URLs firmadas
  reg.t_inicio = Date.now();
  const r1 = http.post(
    `${API}/uploads`,
    JSON.stringify({
      original_name: p.archivo,
      size_bytes: p.tamano_bytes,
      kind: p.kind,
      declared_mime: p.mime,
      checksum_sha256: p.sha256,
    }),
    { headers: auth, tags: tags(OP.init, 'control'), responseType: 'text' },
  );
  registrar429(r1);
  if (!check(r1, { 'init 2xx': (r) => r.status >= 200 && r.status < 300 })) return fallar('init', r1);
  const init = r1.json();
  mFirma.add(r1.timings.duration, { perfil: pid });
  reg.asset_id = init.asset_id;
  reg.upload_id = init.upload_id;
  reg.partes = init.total_parts;
  reg.api_firma_ms = r1.timings.duration;

  // 2. Transferencia directa de cada parte al almacenamiento
  const archivo = ARCHIVOS[pid];
  const partes = [...init.parts].sort((a, b) => a.part_number - b.part_number);
  const t0 = Date.now();
  reg.t_transfer_inicio = t0;
  for (const parte of partes) {
    const offset = (parte.part_number - 1) * init.part_size;
    const largo = Math.min(init.part_size, p.tamano_bytes - offset);
    const buf = new Uint8Array(largo);
    await archivo.seek(offset, SeekMode.Start);
    let leidos = 0;
    while (leidos < largo) {
      const n = await archivo.read(buf.subarray(leidos));
      if (n === null || n === 0) break;
      leidos += n;
    }
    const rp = http.put(parte.url, buf.buffer, {
      tags: tags(OP.parte, 'transferencia'),
      headers: { 'Content-Type': 'application/octet-stream' },
    });
    if (!check(rp, { 'parte 200': (r) => r.status === 200 })) return fallar(`parte_${parte.part_number}`, rp);
  }
  const transf = Date.now() - t0;
  reg.transferencia_ms = transf;
  mTransf.add(transf, { perfil: pid });
  mTransfMbps.add((p.tamano_bytes * 8) / 1e6 / Math.max(transf / 1000, 0.001), { perfil: pid });

  // 3. Confirmar la carga completa (la API ensambla y encola media:scan)
  const r3 = http.post(`${API}/uploads/${init.upload_id}/complete`, null, {
    headers: auth,
    tags: tags(OP.complete, 'control'),
  });
  registrar429(r3);
  if (!check(r3, { 'complete 2xx': (r) => r.status >= 200 && r.status < 300 })) return fallar('complete', r3);
  const tComp = Date.now();
  reg.t_confirmado = tComp;
  reg.confirmacion_ms = r3.timings.duration;
  mConfirma.add(r3.timings.duration, { perfil: pid });

  // 4. Crear el recurso en la unidad del profesor
  const rtipo = p.kind === 'audio' ? 'audio' : 'video';
  const r4 = http.post(
    `${API}/units/${prof.unit_id}/resources`,
    JSON.stringify({ type: rtipo, title: `k6 ${pid} #${it}`, required: false, asset_id: init.asset_id }),
    { headers: auth, tags: tags(OP.recurso, 'control') },
  );
  registrar429(r4);
  check(r4, { 'recurso 2xx': (r) => r.status >= 200 && r.status < 300 });

  // 5. Consultar el estado hasta un estado terminal
  let estado = '';
  let info = null;
  while (Date.now() - tComp < TIMEOUT_READY_MS) {
    sleep(POLL_S);
    const r = http.get(`${API}/assets/${init.asset_id}`, {
      headers: auth,
      tags: tags(OP.estado, 'control'),
      responseType: 'text',
    });
    registrar429(r);
    if (r.status !== 200) continue; // un 429 o un error puntual no detiene la observacion
    info = r.json();
    estado = info.status;
    const ahora = Date.now();
    if (!reg[`t_visto_${estado}`]) reg[`t_visto_${estado}`] = ahora;
    if (estado === 'processing' && !reg.espera_hasta_procesar_ms) {
      reg.espera_hasta_procesar_ms = ahora - tComp;
      mHastaProc.add(ahora - tComp, { perfil: pid });
    }
    if (['ready', 'failed', 'infected'].includes(estado)) break;
  }
  if (estado !== 'ready') {
    reg.ultimo_estado = estado || 'desconocido';
    return fallar(estado ? `estado_${estado}` : 'timeout', null);
  }
  reg.t_ready = reg.t_visto_ready;
  reg.carga_a_ready_ms = reg.t_ready - tComp;
  reg.extremo_a_extremo_ms = reg.t_ready - reg.t_inicio;
  mReady.add(reg.carga_a_ready_ms, { perfil: pid });
  mE2E.add(reg.extremo_a_extremo_ms, { perfil: pid });

  // 6. Validacion funcional: un 200 no basta, el derivado debe ser correcto
  const master = http.get(`${API}/assets/${init.asset_id}/playlist`, {
    headers: auth,
    tags: tags(OP.masterProf, 'control'),
    responseType: 'text',
  });
  const variantes = master.status === 200 ? (master.body.match(/playlist\?file=/g) || []).length : -1;
  reg.duracion_detectada_s = info.duration_secs;
  reg.variantes = variantes;
  const valido = check(null, {
    'hls_available': () => info.hls_available === true,
    'duracion coincide (+-2 s)': () => Math.abs(info.duration_secs - p.duracion_s) <= 2,
    'rendiciones esperadas': () => variantes === p.rendiciones_esperadas.length,
  });
  if (!valido) return fallar('validacion', master);

  reg.estado_final = 'ready';
  mExito.add(true, { perfil: pid });
  console.log('E2_CARGA ' + JSON.stringify(reg));
}

// =====================================================================
// ESPECTADOR
// =====================================================================
function lineasUrl(m3u8) {
  return m3u8.split('\n').map((l) => l.trim()).filter((l) => l && !l.startsWith('#'));
}

export function espectador() {
  const vu = exec.vu.idInTest;
  const est = SEED.estudiantes[(vu - 1) % SEED.estudiantes.length];
  const assets = SEED.curso_contenido.assets;
  // Cada espectador rota por los contenidos (P1, P2, P3, A1) sesion a sesion.
  const a = assets[(vu - 1 + exec.vu.iterationInScenario) % assets.length];
  const auth = { Authorization: `Bearer ${est.token}` };
  const f = fase();
  const tags = (op, tipo) => ({ name: op, tipo, perfil: a.perfil, fase: f });

  // 1. Master
  const rm = http.get(`${API}/assets/${a.asset_id}/playlist`, {
    headers: auth,
    tags: tags(OP.master, 'consumo_api'),
    responseType: 'text',
  });
  registrar429(rm);
  mManifiesto.add(rm.timings.duration, { tipo_lista: 'maestro', perfil: a.perfil, fase: f });
  if (!check(rm, { 'master 200 + #EXTM3U': (r) => r.status === 200 && r.body.startsWith('#EXTM3U') })) {
    sleep(C.pausa_entre_videos_s);
    return;
  }
  const variantes = lineasUrl(rm.body);
  // Rendicion elegida: rota por usuario virtual para mezclar calidades (declarado).
  const urlVar = variantes[(vu - 1) % variantes.length];
  const rendicion = (urlVar.match(/file=([^&]+)\.m3u8/) || [])[1] || '?';

  // 2. Lista de la rendicion (segmentos con URL firmada)
  const rv = http.get(urlVar, { headers: auth, tags: tags(OP.variante, 'consumo_api'), responseType: 'text' });
  registrar429(rv);
  mManifiesto.add(rv.timings.duration, { tipo_lista: 'variante', perfil: a.perfil, fase: f });
  if (!check(rv, { 'variante 200 + #EXTM3U': (r) => r.status === 200 && r.body.startsWith('#EXTM3U') })) {
    sleep(C.pausa_entre_videos_s);
    return;
  }
  const segs = [];
  let dur = 0;
  for (const l of rv.body.split('\n')) {
    const t = l.trim();
    if (t.startsWith('#EXTINF:')) dur = parseFloat(t.slice(8));
    else if (t && !t.startsWith('#')) segs.push({ url: t, dur });
  }

  // 3. Reproduccion simulada (ver cabecera). acum[k] = inicio del segmento k en la linea de tiempo.
  const B = Number(C.prebuffer_segmentos);
  const n = Math.min(segs.length, Number(C.segmentos_por_sesion));
  const acum = [0];
  for (let k = 0; k < n; k++) acum.push(acum[k] + segs[k].dur);
  let inicioReproduccion = null;
  for (let i = 0; i < n; i++) {
    if (inicioReproduccion !== null && i >= B) {
      // Se pide el segmento i cuando la reproduccion llega al inicio del segmento i-B.
      const objetivo = inicioReproduccion + acum[i - B] * 1000;
      const espera = objetivo - Date.now();
      if (espera > 0) sleep(espera / 1000);
    }
    const ft = fase();
    const rs = http.get(segs[i].url, {
      tags: { name: OP.segmento, tipo: 'consumo', perfil: a.perfil, rendicion, fase: ft },
    });
    const bytes = Number(rs.headers['Content-Length'] || 0);
    const ok = check(rs, { 'segmento 200 con datos': (r) => r.status === 200 && bytes > 0 });
    mSegmento.add(rs.timings.duration, { perfil: a.perfil, rendicion, fase: ft });
    if (ok && rs.timings.duration > 0) {
      mSegMbps.add((bytes * 8) / 1e6 / (rs.timings.duration / 1000), { perfil: a.perfil, rendicion, fase: ft });
    }
    const fin = Date.now();
    if (inicioReproduccion === null && (i === B - 1 || i === n - 1)) inicioReproduccion = fin; // arranca tras la pre-carga
    if (inicioReproduccion !== null && i >= B) {
      // Riesgo de corte: el segmento termino de llegar despues del momento en que
      // la reproduccion lo necesitaba. Es una ESTIMACION (no es un reproductor real).
      mCorte.add(fin > inicioReproduccion + acum[i] * 1000, { perfil: a.perfil, rendicion, fase: ft });
    }
  }
  sleep(C.pausa_entre_videos_s);
}
