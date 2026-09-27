// =====================================================================
// Entrega 2 - Analisis de capacidad - ESCENARIO 1
// Actividad academica concurrente: estudiantes que consultan el catalogo,
// acceden a un curso, consultan contenido, registran progreso valido y
// presentan quizzes. Mezcla de lecturas y escrituras, cada usuario virtual
// con su propia cuenta e intentos distintos (evita conflictos artificiales).
//
// Herramienta: k6 (https://k6.io). Se eligio por: modela recorridos
// multi-paso con validaciones (check), controla el patron de inyeccion con
// executors (tasa de llegada creciente), define criterios de exito/parada
// con thresholds y exporta resultados reproducibles (JSON).
//
// USO LOCAL (para depurar el recorrido; los numeros NO son de capacidad
// porque el generador comparte maquina con la app):
//   k6 run capacity-planning/k6/escenario1_actividad_academica.js
//
// USO EN LA NUBE (numeros validos del informe): ejecutar k6 desde una
// maquina FUERA de Web Server y Worker Server, apuntando a la URL publica:
//   BASE_URL=https://<web-server>/api/v1 k6 run --out json=resultados/e1.json \
//     capacity-planning/k6/escenario1_actividad_academica.js
//
// Todo se parametriza por variables de entorno; local -> nube es solo
// cambiar BASE_URL. Ver capacity-planning/README.md.
// =====================================================================

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Trend, Counter, Rate } from 'k6/metrics';

// ---------------------------------------------------------------------
// Configuracion (todo overridable por entorno)
// ---------------------------------------------------------------------
const BASE = __ENV.BASE_URL || 'http://localhost:8080/api/v1';
const ADMIN_EMAIL = __ENV.ADMIN_EMAIL || 'admin@mooc.local';
const ADMIN_PASSWORD = __ENV.ADMIN_PASSWORD || 'Admin123!';

// Cantidad de estudiantes sinteticos que se preparan ANTES de la corrida.
// Ojo: el login es publico y esta limitado a 30/min por IP (no configurable),
// por eso el valor por defecto (20) mantiene el setup por debajo de ese tope.
// Para N mayores, sembrar en BD o espaciar el arranque (ver README).
const N_STUDENTS = parseInt(__ENV.N_STUDENTS || '20', 10);

// Escala global de la tasa de llegada. RATE_SCALE=2 duplica todos los niveles.
const RATE_SCALE = parseFloat(__ENV.RATE_SCALE || '1');
const rate = (x) => Math.max(1, Math.round(x * RATE_SCALE));

// Preguntas del quiz sintetico. La opcion correcta SIEMPRE empieza por 'OK-'
// y las distractoras por 'NO-', asi el VU responde correcto sin conocer la
// clave (que nunca sale al cliente): elige la opcion cuyo texto empieza 'OK-'.
const N_PREGUNTAS = 5;

// ---------------------------------------------------------------------
// Metricas propias: separan fallo funcional real de rechazo esperado.
// El enunciado exige distinguir 429/limite de tasa y rechazos de negocio
// de las solicitudes validas que fallan.
// ---------------------------------------------------------------------
const erroresFuncionales = new Counter('errores_funcionales');   // fallos reales (5xx, estado inesperado)
const rechazosNegocio = new Counter('rechazos_negocio');         // 4xx esperados por reglas de negocio
const tasaRateLimited = new Rate('tasa_rate_limited');           // proporcion de 429
const quizCalificadoOk = new Rate('quiz_calificado_correcto');   // el submit devolvio la nota esperada
const dupSinDobleCalif = new Rate('idempotencia_sin_doble_calificacion');
const tProgreso = new Trend('t_progreso_ms', true);
const tQuizSubmit = new Trend('t_quiz_submit_ms', true);

// ---------------------------------------------------------------------
// Definicion de la carga: linea base + 3 niveles crecientes + repeticion
// cerca del limite (para comprobar estabilidad) + enfriamiento.
// Modelo abierto (tasa de llegada) para caracterizar throughput y saturacion.
// ---------------------------------------------------------------------
export const options = {
  // Solo cuentan como "fallo" los estados fuera de 2xx que NO sean 429.
  // (429 se contabiliza aparte en tasa_rate_limited.)
  scenarios: {
    // Prueba de integridad: envio duplicado sin doble calificacion.
    // Corre una sola vez al inicio, con 1 VU, sin sumar carga.
    integridad_idempotencia: {
      executor: 'per-vu-iterations',
      vus: 1,
      iterations: 1,
      exec: 'integridadIdempotencia',
      startTime: '0s',
      tags: { scenario: 'integridad' },
    },
    // Carga academica principal.
    carga_academica: {
      executor: 'ramping-arrival-rate',
      startRate: rate(5),
      timeUnit: '1s',
      preAllocatedVUs: parseInt(__ENV.PRE_VUS || '50', 10),
      maxVUs: parseInt(__ENV.MAX_VUS || '300', 10),
      startTime: '10s', // deja terminar el chequeo de integridad
      stages: [
        { target: rate(5), duration: '1m' },  // nivel 0 - linea base
        { target: rate(20), duration: '2m' }, // nivel 1
        { target: rate(50), duration: '2m' }, // nivel 2
        { target: rate(90), duration: '2m' }, // nivel 3 - cerca del limite
        { target: rate(90), duration: '2m' }, // repeticion cerca del limite (estabilidad)
        { target: rate(5), duration: '1m' },  // enfriamiento
      ],
      exec: 'actividadAcademica',
      tags: { scenario: 'academica' },
    },
  },
  thresholds: {
    // Latencia de la actividad academica (ajustar los umbrales al criterio
    // de exito que declares en el informe).
    'http_req_duration{scenario:academica}': ['p(95)<800', 'p(99)<2000'],
    // Fallos funcionales reales: idealmente cero.
    errores_funcionales: ['count<1'],
    // Los checks funcionales deben pasar casi siempre.
    checks: ['rate>0.98'],
    // La calificacion del quiz debe ser correcta cuando el submit responde 200.
    quiz_calificado_correcto: ['rate>0.99'],
    // El duplicado nunca debe recalificar.
    idempotencia_sin_doble_calificacion: ['rate>0.99'],
  },
  setupTimeout: '180s',
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

// ---------------------------------------------------------------------
// Utilidades
// ---------------------------------------------------------------------
function jsonHeaders(token) {
  const h = { 'Content-Type': 'application/json' };
  if (token) h['Authorization'] = `Bearer ${token}`;
  return h;
}

function parse(res) {
  try { return res.json(); } catch (_e) { return null; }
}

// login con reintento: el scope publico esta limitado a 30/min por IP.
function login(email, password) {
  for (let intento = 0; intento < 6; intento++) {
    const res = http.post(`${BASE}/auth/login`,
      JSON.stringify({ email, password }),
      { headers: jsonHeaders(), tags: { name: 'setup_login' } });
    if (res.status === 200) {
      const b = parse(res);
      if (b && b.access_token) return b.access_token;
    }
    if (res.status === 429) { sleep(2.5); continue; } // esperar la ventana
    sleep(1);
  }
  throw new Error(`login fallido para ${email}`);
}

// Registra el resultado de una peticion clasificando el tipo de fallo.
// Devuelve el objeto response para inspeccion adicional.
function clasificar(res, name, esEscritura) {
  const ok = res.status >= 200 && res.status < 300;
  tasaRateLimited.add(res.status === 429);
  if (!ok) {
    if (res.status === 429) {
      // limite de tasa: rechazo esperado, no fallo funcional.
    } else if (res.status >= 400 && res.status < 500) {
      // 4xx: rechazo de negocio (p. ej. intentos agotados, secuencia invalida).
      rechazosNegocio.add(1, { name });
    } else {
      // 5xx u otro: fallo real.
      erroresFuncionales.add(1, { name });
    }
  }
  return ok;
}

// ---------------------------------------------------------------------
// setup(): se ejecuta UNA vez. Crea datos sinteticos reproducibles:
//   - un profesor y un curso publicado (1 modulo, 3 unidades de texto
//     obligatorio + un quiz obligatorio de N_PREGUNTAS)
//   - N estudiantes activos, ya autenticados e inscritos
// Las sesiones se preparan ANTES de la corrida (la autenticacion NO forma
// parte del recorrido medido; la rafaga de login se mide como variante
// separada, ver README).
// ---------------------------------------------------------------------
export function setup() {
  const stamp = Date.now();
  const adminToken = login(ADMIN_EMAIL, ADMIN_PASSWORD);

  // --- Profesor ---
  const teacherEmail = `e2profe${stamp}@mooc.local`;
  const teacherPass = 'Profesor2026';
  http.post(`${BASE}/admin/users`,
    JSON.stringify({ email: teacherEmail, full_name: 'Profe Carga E2', password: teacherPass, role: 'teacher' }),
    { headers: jsonHeaders(adminToken), tags: { name: 'setup_admin_user' } });
  const teacherToken = login(teacherEmail, teacherPass);

  // --- Curso + version ---
  const course = parse(http.post(`${BASE}/courses`,
    JSON.stringify({ title: `Curso carga E1 ${stamp}`, summary: 'Curso sintetico para prueba de capacidad', category: 'cloud', level: 'beginner' }),
    { headers: jsonHeaders(teacherToken), tags: { name: 'setup_course' } }));
  const slug = course.slug;
  const versionId = course.draft_version.id;

  // --- 1 modulo, 3 unidades de texto obligatorio ---
  const moduleId = parse(http.post(`${BASE}/versions/${versionId}/modules`,
    JSON.stringify({ title: 'Modulo 1' }),
    { headers: jsonHeaders(teacherToken), tags: { name: 'setup_module' } })).id;

  let quizUnitId = null;
  for (let u = 1; u <= 3; u++) {
    const unitId = parse(http.post(`${BASE}/modules/${moduleId}/units`,
      JSON.stringify({ title: `Unidad ${u}` }),
      { headers: jsonHeaders(teacherToken), tags: { name: 'setup_unit' } })).id;
    http.post(`${BASE}/units/${unitId}/resources`,
      JSON.stringify({ type: 'rich_text', title: `Lectura ${u}`, content_md: `# Unidad ${u}\n\nContenido sintetico.`, required: true }),
      { headers: jsonHeaders(teacherToken), tags: { name: 'setup_resource' } });
    quizUnitId = unitId;
  }

  // --- Quiz obligatorio en la ultima unidad ---
  const quizRes = parse(http.post(`${BASE}/units/${quizUnitId}/resources`,
    JSON.stringify({ type: 'quiz', title: 'Quiz de cierre', required: true }),
    { headers: jsonHeaders(teacherToken), tags: { name: 'setup_quiz' } }));
  const quizId = quizRes.quiz_id;

  // Subir max_attempts para que las iteraciones no agoten intentos (por
  // defecto son 3). Esto queda registrado como parte de la config de la corrida.
  http.patch(`${BASE}/quizzes/${quizId}`,
    JSON.stringify({ max_attempts: 100000 }),
    { headers: jsonHeaders(teacherToken), tags: { name: 'setup_quiz_patch' } });

  // Preguntas: 1 correcta ('OK-...') + 3 distractoras ('NO-...').
  for (let q = 1; q <= N_PREGUNTAS; q++) {
    http.post(`${BASE}/quizzes/${quizId}/questions`,
      JSON.stringify({
        prompt: `Pregunta ${q}`,
        options: [
          { text: `OK-${q}`, is_correct: true },
          { text: `NO-${q}-a`, is_correct: false },
          { text: `NO-${q}-b`, is_correct: false },
          { text: `NO-${q}-c`, is_correct: false },
        ],
      }),
      { headers: jsonHeaders(teacherToken), tags: { name: 'setup_question' } });
  }

  // --- Publicar ---
  http.post(`${BASE}/versions/${versionId}/publish`, '{}',
    { headers: jsonHeaders(teacherToken), tags: { name: 'setup_publish' } });

  // --- N estudiantes activos, autenticados e inscritos ---
  const students = [];
  for (let i = 0; i < N_STUDENTS; i++) {
    const email = `e2est${stamp}_${i}@mooc.local`;
    const pass = 'Estudiante2026';
    http.post(`${BASE}/admin/users`,
      JSON.stringify({ email, full_name: `Est ${i}`, password: pass, role: 'student' }),
      { headers: jsonHeaders(adminToken), tags: { name: 'setup_admin_user' } });
    const token = login(email, pass);
    const enr = parse(http.post(`${BASE}/enrollments`,
      JSON.stringify({ slug }),
      { headers: jsonHeaders(token), tags: { name: 'setup_enroll' } }));
    students.push({ email, token, enrollmentId: enr.id });
  }

  // --- Sondear el outline de un estudiante para obtener los stable_id de
  //     los recursos consumibles (no-quiz) y confirmar el quiz_id. ---
  const outline = parse(http.get(`${BASE}/enrollments/${students[0].enrollmentId}/outline`,
    { headers: jsonHeaders(students[0].token), tags: { name: 'setup_outline' } }));
  const consumibles = [];
  for (const m of outline.modules || []) {
    for (const u of m.units || []) {
      for (const r of u.resources || []) {
        if (r.type !== 'quiz' && r.stable_id) consumibles.push(r.stable_id);
      }
    }
  }

  console.log(`setup listo: curso=${slug} quiz=${quizId} estudiantes=${students.length} consumibles=${consumibles.length}`);
  return { slug, quizId, consumibles, students };
}

// ---------------------------------------------------------------------
// Recorrido academico principal (una iteracion = una sesion plausible).
// ---------------------------------------------------------------------
export function actividadAcademica(data) {
  const s = data.students[Math.floor(Math.random() * data.students.length)];
  const h = jsonHeaders(s.token);

  // 1) Explorar el catalogo (lecturas publicas).
  group('catalogo', () => {
    const list = http.get(`${BASE}/catalog`, { headers: h, tags: { name: 'catalog_list' } });
    clasificar(list, 'catalog_list', false);
    check(list, { 'catalogo 200': (r) => r.status === 200 });

    const detail = http.get(`${BASE}/catalog/${data.slug}`, { headers: h, tags: { name: 'catalog_detail' } });
    clasificar(detail, 'catalog_detail', false);
    check(detail, { 'detalle 200': (r) => r.status === 200 });
  });

  sleep(1 + Math.random());

  // 2) Consultar contenido y registrar progreso valido (escritura).
  group('contenido', () => {
    const outline = http.get(`${BASE}/enrollments/${s.enrollmentId}/outline`,
      { headers: h, tags: { name: 'outline' } });
    clasificar(outline, 'outline', false);
    check(outline, { 'outline 200': (r) => r.status === 200 });

    if (data.consumibles.length > 0) {
      const stable = data.consumibles[Math.floor(Math.random() * data.consumibles.length)];
      const t0 = Date.now();
      // Solo recursos NO-quiz reciben eventos; el quiz avanza al enviarse.
      // delta_secs debe superar el tiempo minimo de permanencia del recurso
      // (rich_text exige 15s); por debajo, 'complete' responde 422.
      for (const ev of [
        { event_type: 'open' },
        { event_type: 'heartbeat', delta_secs: 20 },
        { event_type: 'complete' },
      ]) {
        const body = Object.assign({ enrollment_id: s.enrollmentId, resource_stable_id: stable }, ev);
        const pr = http.post(`${BASE}/progress/events`, JSON.stringify(body),
          { headers: h, tags: { name: 'progress_event' } });
        clasificar(pr, 'progress_event', true);
      }
      tProgreso.add(Date.now() - t0);
    }
  });

  sleep(1 + Math.random());

  // 3) Presentar un quiz (intento nuevo cada vez -> intentos distintos).
  group('quiz', () => {
    const start = http.post(`${BASE}/quizzes/${data.quizId}/attempts`, null,
      { headers: h, tags: { name: 'quiz_start' } });
    if (!clasificar(start, 'quiz_start', true)) return;
    const attempt = parse(start);
    if (!attempt || !attempt.questions) return;

    // Responder correcto: elegir la opcion cuyo texto empieza por 'OK-'.
    const answers = {};
    for (const q of attempt.questions) {
      const correcta = (q.options || []).find((o) => (o.text || '').indexOf('OK-') === 0);
      if (correcta) answers[q.stable_id] = [correcta.stable_id];
    }

    const t0 = Date.now();
    const submit = http.post(`${BASE}/attempts/${attempt.id}/submit`,
      JSON.stringify({ answers }),
      { headers: Object.assign({ 'Idempotency-Key': `e1-${attempt.id}` }, h), tags: { name: 'quiz_submit' } });
    tQuizSubmit.add(Date.now() - t0);
    const okSubmit = clasificar(submit, 'quiz_submit', true);

    if (okSubmit) {
      const b = parse(submit);
      // Validacion funcional: un 200 no basta; verificar la nota esperada.
      const correcto = b && b.passed === true && b.score === 100;
      quizCalificadoOk.add(correcto);
      check(b, { 'quiz calificado 100/aprobado': () => correcto });
    }
  });

  sleep(1 + Math.random() * 2);
}

// ---------------------------------------------------------------------
// Integridad: envio duplicado sin doble calificacion.
// Envia el MISMO intento dos veces con la MISMA Idempotency-Key y verifica
// que la segunda respuesta sea identica (la idempotencia devuelve la
// respuesta cacheada, no recalifica). Complementar en el informe con un
// conteo en BD de intentos calificados.
// ---------------------------------------------------------------------
export function integridadIdempotencia(data) {
  const s = data.students[0];
  const h = jsonHeaders(s.token);

  const attempt = parse(http.post(`${BASE}/quizzes/${data.quizId}/attempts`, null,
    { headers: h, tags: { name: 'integ_quiz_start' } }));
  if (!attempt || !attempt.questions) { dupSinDobleCalif.add(false); return; }

  const answers = {};
  for (const q of attempt.questions) {
    const correcta = (q.options || []).find((o) => (o.text || '').indexOf('OK-') === 0);
    if (correcta) answers[q.stable_id] = [correcta.stable_id];
  }
  const key = `integridad-${attempt.id}`;
  const hk = Object.assign({ 'Idempotency-Key': key }, h);

  const r1 = parse(http.post(`${BASE}/attempts/${attempt.id}/submit`, JSON.stringify({ answers }),
    { headers: hk, tags: { name: 'integ_submit_1' } }));
  const r2 = parse(http.post(`${BASE}/attempts/${attempt.id}/submit`, JSON.stringify({ answers }),
    { headers: hk, tags: { name: 'integ_submit_2' } }));

  const identico = !!(r1 && r2 && r1.score === r2.score && r1.passed === r2.passed);
  dupSinDobleCalif.add(identico);
  check(null, { 'duplicado no recalifica (respuesta identica)': () => identico });
  console.log(`integridad: submit1 score=${r1 && r1.score} submit2 score=${r2 && r2.score} identico=${identico}`);
}

// ---------------------------------------------------------------------
// Guarda un resumen JSON reproducible junto al texto en consola.
// ---------------------------------------------------------------------
export function handleSummary(data) {
  return {
    stdout: textSummary(data),
    'capacity-planning/resultados/escenario1_summary.json': JSON.stringify(data, null, 2),
  };
}

// Resumen de texto minimo (evita dependencias externas de jslib).
function textSummary(data) {
  const m = data.metrics || {};
  const g = (name, stat) => {
    const v = m[name] && m[name].values;
    if (!v) return 'n/a';
    return (v[stat] !== undefined ? v[stat] : v.count !== undefined ? v.count : v.rate);
  };
  const dur = (m['http_req_duration'] && m['http_req_duration'].values) || {};
  return [
    '',
    '==== Escenario 1 - resumen ====',
    `throughput (reqs):        ${g('http_reqs', 'count')} (${(m['http_reqs'] && m['http_reqs'].values && m['http_reqs'].values.rate || 0).toFixed(2)}/s)`,
    `http_req_duration p50:    ${(dur['med'] || 0).toFixed(1)} ms`,
    `http_req_duration p95:    ${(dur['p(95)'] || 0).toFixed(1)} ms`,
    `http_req_duration p99:    ${(dur['p(99)'] || 0).toFixed(1)} ms`,
    `errores_funcionales:      ${g('errores_funcionales', 'count')}`,
    `rechazos_negocio:         ${g('rechazos_negocio', 'count')}`,
    `tasa_rate_limited:        ${((m['tasa_rate_limited'] && m['tasa_rate_limited'].values && m['tasa_rate_limited'].values.rate || 0) * 100).toFixed(2)}%`,
    `quiz_calificado_correcto: ${((m['quiz_calificado_correcto'] && m['quiz_calificado_correcto'].values && m['quiz_calificado_correcto'].values.rate || 0) * 100).toFixed(2)}%`,
    `idempotencia_ok:          ${((m['idempotencia_sin_doble_calificacion'] && m['idempotencia_sin_doble_calificacion'].values && m['idempotencia_sin_doble_calificacion'].values.rate || 0) * 100).toFixed(2)}%`,
    '================================',
    '',
  ].join('\n');
}
