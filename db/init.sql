-- =====================================================================
-- Esquema de la plataforma MOOC
--
-- PostgreSQL lo ejecuta automaticamente la PRIMERA vez que se crea el
-- volumen de datos (docker-entrypoint-initdb.d). Si cambias este archivo
-- debes borrar el volumen:  docker compose down -v
--
-- Principio de diseno: Postgres es la FUENTE DE VERDAD. Redis solo guarda
-- copias efimeras (cache, rate limit, cola). Ningun binario vive aqui:
-- los archivos estan en el almacenamiento de objetos y esta base solo
-- guarda la llave del objeto.
-- =====================================================================

-- ---------------------------------------------------------------------
-- 1. IDENTIDAD
-- ---------------------------------------------------------------------

-- Roles globales del sistema. El enunciado exige exactamente tres.
CREATE TYPE user_role   AS ENUM ('admin', 'teacher', 'student');
-- pending_verification: se registro pero no confirmo el correo.
CREATE TYPE user_status AS ENUM ('pending_verification', 'active', 'suspended');

CREATE TABLE users (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email             TEXT        NOT NULL UNIQUE,
    password_hash     TEXT        NOT NULL,
    full_name         TEXT        NOT NULL,
    role              user_role   NOT NULL DEFAULT 'student',
    status            user_status NOT NULL DEFAULT 'pending_verification',
    email_verified_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sesiones revocables. Guardamos el SHA-256 del token, nunca el token.
-- Si alguien lee la base de datos no puede suplantar a nadie.
CREATE TABLE sessions (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT        NOT NULL UNIQUE,
    user_agent   TEXT        NOT NULL DEFAULT '',
    ip           TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX idx_sessions_user ON sessions(user_id) WHERE revoked_at IS NULL;

-- Tokens de un solo uso para verificar correo y restablecer contrasena.
CREATE TYPE token_purpose AS ENUM ('verify_email', 'reset_password');

CREATE TABLE auth_tokens (
    id         UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT          NOT NULL UNIQUE,
    purpose    token_purpose NOT NULL,
    expires_at TIMESTAMPTZ   NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- Bitacora de auditoria. Solo INSERT: nunca se actualiza ni se borra.
CREATE TABLE audit_log (
    id          BIGSERIAL   PRIMARY KEY,
    actor_id    UUID        REFERENCES users(id) ON DELETE SET NULL,
    action      TEXT        NOT NULL,
    entity_type TEXT        NOT NULL DEFAULT '',
    entity_id   TEXT        NOT NULL DEFAULT '',
    metadata    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    ip          TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_created ON audit_log(created_at DESC);
CREATE INDEX idx_audit_actor   ON audit_log(actor_id);

-- ---------------------------------------------------------------------
-- 2. ALMACENAMIENTO DE OBJETOS (metadatos de los binarios)
-- ---------------------------------------------------------------------

CREATE TYPE asset_kind AS ENUM ('video', 'audio', 'pdf', 'image', 'file');
-- Ciclo de vida: awaiting_upload -> uploaded -> scanning -> clean
--                -> processing -> ready   (o infected / failed)
CREATE TYPE asset_status AS ENUM (
    'awaiting_upload', 'uploaded', 'scanning', 'infected',
    'processing', 'ready', 'failed'
);

CREATE TABLE assets (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id        UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind            asset_kind   NOT NULL,
    status          asset_status NOT NULL DEFAULT 'awaiting_upload',
    original_name   TEXT         NOT NULL,
    declared_mime   TEXT         NOT NULL DEFAULT '',
    detected_mime   TEXT         NOT NULL DEFAULT '',
    size_bytes      BIGINT       NOT NULL DEFAULT 0,
    checksum_sha256 TEXT         NOT NULL DEFAULT '',
    object_key      TEXT         NOT NULL,             -- llave del ORIGINAL, se conserva siempre
    hls_prefix      TEXT         NOT NULL DEFAULT '',  -- prefijo del derivado HLS
    duration_secs   INTEGER      NOT NULL DEFAULT 0,
    failure_reason  TEXT         NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_assets_owner ON assets(owner_id);

-- Carga multipart reanudable. upload_id es el identificador que devuelve
-- el almacenamiento de objetos; las partes confirmadas se acumulan aqui
-- para poder reanudar la carga dentro de la ventana de 24 horas.
CREATE TABLE uploads (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id     UUID        NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    owner_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    upload_id    TEXT        NOT NULL,
    total_parts  INTEGER     NOT NULL,
    part_size    BIGINT      NOT NULL,
    parts        JSONB       NOT NULL DEFAULT '[]'::jsonb, -- [{part_number, etag, size}]
    expires_at   TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    aborted_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_uploads_owner ON uploads(owner_id);

-- ---------------------------------------------------------------------
-- 3. AUTORIA: Curso -> Version -> Modulo -> Unidad -> Recurso
-- ---------------------------------------------------------------------

CREATE TYPE course_status  AS ENUM ('draft', 'published', 'unpublished', 'archived');
CREATE TYPE version_status AS ENUM ('draft', 'published', 'superseded');

CREATE TABLE courses (
    id                 UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    slug               TEXT          NOT NULL UNIQUE,
    title              TEXT          NOT NULL,
    summary            TEXT          NOT NULL DEFAULT '',
    owner_id           UUID          NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status             course_status NOT NULL DEFAULT 'draft',
    language           TEXT          NOT NULL DEFAULT 'es',
    level              TEXT          NOT NULL DEFAULT 'beginner',
    category           TEXT          NOT NULL DEFAULT '',
    tags               TEXT[]        NOT NULL DEFAULT '{}',
    current_version_id UUID,          -- version publicada vigente
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX idx_courses_owner  ON courses(owner_id);
CREATE INDEX idx_courses_status ON courses(status);

CREATE TABLE course_versions (
    id                 UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id          UUID           NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    version_number     INTEGER        NOT NULL,
    status             version_status NOT NULL DEFAULT 'draft',
    -- Criterios de aprobacion, obligatorios para poder publicar.
    passing_score      NUMERIC(5,2)   NOT NULL DEFAULT 70.00,  -- nota minima de quizzes
    required_completion NUMERIC(5,2)  NOT NULL DEFAULT 100.00, -- % de recursos obligatorios
    published_at       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT now(),
    UNIQUE (course_id, version_number)
);

-- stable_id: identificador que SOBREVIVE a las nuevas versiones. El progreso
-- del estudiante apunta a este valor y no al id fisico de la fila, de modo
-- que publicar una version nueva no borra lo que el estudiante ya hizo.
CREATE TABLE modules (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id UUID        NOT NULL REFERENCES course_versions(id) ON DELETE CASCADE,
    stable_id  UUID        NOT NULL,
    title      TEXT        NOT NULL,
    position   INTEGER     NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (version_id, stable_id),
    UNIQUE (version_id, position) DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE units (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id  UUID        NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    stable_id  UUID        NOT NULL,
    title      TEXT        NOT NULL,
    position   INTEGER     NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (module_id, position) DEFERRABLE INITIALLY DEFERRED
);

CREATE TYPE resource_type AS ENUM (
    'rich_text', 'image', 'video', 'audio', 'pdf', 'download', 'external_link', 'quiz'
);
CREATE TYPE resource_processing AS ENUM ('not_applicable', 'pending', 'processing', 'ready', 'failed');

CREATE TABLE resources (
    id                UUID                PRIMARY KEY DEFAULT gen_random_uuid(),
    unit_id           UUID                NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    stable_id         UUID                NOT NULL,
    type              resource_type       NOT NULL,
    title             TEXT                NOT NULL,
    position          INTEGER             NOT NULL,
    visible           BOOLEAN             NOT NULL DEFAULT TRUE,
    downloadable      BOOLEAN             NOT NULL DEFAULT FALSE,
    required          BOOLEAN             NOT NULL DEFAULT TRUE,
    content_md        TEXT                NOT NULL DEFAULT '', -- Markdown extendido canonico
    content_revision  INTEGER             NOT NULL DEFAULT 0,  -- contador de autosave
    external_url      TEXT                NOT NULL DEFAULT '',
    asset_id          UUID                REFERENCES assets(id) ON DELETE SET NULL,
    processing_status resource_processing NOT NULL DEFAULT 'not_applicable',
    created_at        TIMESTAMPTZ         NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ         NOT NULL DEFAULT now(),
    UNIQUE (unit_id, position) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX idx_resources_unit ON resources(unit_id);

-- ---------------------------------------------------------------------
-- 4. EVALUACION (solo seleccion multiple de texto en esta entrega)
-- ---------------------------------------------------------------------

CREATE TYPE feedback_policy AS ENUM ('none', 'on_submit', 'after_passing');

CREATE TABLE quizzes (
    id                UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_id       UUID            NOT NULL UNIQUE REFERENCES resources(id) ON DELETE CASCADE,
    max_attempts      INTEGER         NOT NULL DEFAULT 3,
    time_limit_secs   INTEGER         NOT NULL DEFAULT 0,   -- 0 = sin limite
    passing_score     NUMERIC(5,2)    NOT NULL DEFAULT 70.00,
    shuffle_questions BOOLEAN         NOT NULL DEFAULT FALSE,
    feedback          feedback_policy NOT NULL DEFAULT 'on_submit',
    created_at        TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ     NOT NULL DEFAULT now()
);

CREATE TABLE quiz_questions (
    id        UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    quiz_id   UUID         NOT NULL REFERENCES quizzes(id) ON DELETE CASCADE,
    stable_id UUID         NOT NULL,
    prompt    TEXT         NOT NULL,  -- enunciado en texto/Markdown, sin multimedia
    position  INTEGER      NOT NULL,
    points    NUMERIC(5,2) NOT NULL DEFAULT 1.00,
    multiple  BOOLEAN      NOT NULL DEFAULT FALSE, -- permite varias respuestas correctas
    UNIQUE (quiz_id, position) DEFERRABLE INITIALLY DEFERRED
);

-- is_correct NUNCA sale de la base hacia el cliente. Ver quizSnapshot().
CREATE TABLE quiz_options (
    id          UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id UUID    NOT NULL REFERENCES quiz_questions(id) ON DELETE CASCADE,
    stable_id   UUID    NOT NULL,
    text        TEXT    NOT NULL,
    is_correct  BOOLEAN NOT NULL DEFAULT FALSE,
    feedback    TEXT    NOT NULL DEFAULT '',
    position    INTEGER NOT NULL,
    UNIQUE (question_id, position) DEFERRABLE INITIALLY DEFERRED
);

CREATE TYPE attempt_status AS ENUM ('in_progress', 'submitted', 'expired');

CREATE TABLE quiz_attempts (
    id             UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    quiz_id        UUID           NOT NULL REFERENCES quizzes(id) ON DELETE CASCADE,
    user_id        UUID           NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    attempt_number INTEGER        NOT NULL,
    status         attempt_status NOT NULL DEFAULT 'in_progress',
    -- snapshot: copia del cuestionario tal como se le mostro al estudiante.
    -- Si el profesor edita el quiz a mitad del intento, la calificacion sigue
    -- usando la version con la que el estudiante trabajo.
    snapshot       JSONB          NOT NULL,
    answers        JSONB          NOT NULL DEFAULT '{}'::jsonb, -- guardado parcial
    score          NUMERIC(5,2),
    passed         BOOLEAN,
    started_at     TIMESTAMPTZ    NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ,
    submitted_at   TIMESTAMPTZ,
    UNIQUE (quiz_id, user_id, attempt_number)
);
CREATE INDEX idx_attempts_user ON quiz_attempts(user_id, quiz_id);

-- ---------------------------------------------------------------------
-- 5. INSCRIPCION, PROGRESO E INSIGNIAS
-- ---------------------------------------------------------------------

CREATE TYPE enrollment_status AS ENUM ('active', 'withdrawn');
CREATE TYPE learning_state    AS ENUM ('in_progress', 'completed', 'approved');

CREATE TABLE enrollments (
    id           UUID              PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id    UUID              NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id      UUID              NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status       enrollment_status NOT NULL DEFAULT 'active',
    state        learning_state    NOT NULL DEFAULT 'in_progress',
    progress_pct NUMERIC(5,2)      NOT NULL DEFAULT 0.00, -- SIEMPRE calculado en servidor
    enrolled_at  TIMESTAMPTZ       NOT NULL DEFAULT now(),
    withdrawn_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    approved_at  TIMESTAMPTZ,
    UNIQUE (course_id, user_id)   -- reinscribirse reutiliza la fila y su progreso
);

-- Evidencia cruda del avance. Una fila por senal recibida del cliente.
CREATE TABLE progress_events (
    id                 BIGSERIAL   PRIMARY KEY,
    enrollment_id      UUID        NOT NULL REFERENCES enrollments(id) ON DELETE CASCADE,
    resource_stable_id UUID        NOT NULL,
    event_type         TEXT        NOT NULL, -- open | heartbeat | complete
    position_secs      INTEGER     NOT NULL DEFAULT 0,
    delta_secs         INTEGER     NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_enrollment ON progress_events(enrollment_id, created_at DESC);

-- Estado agregado por recurso, derivado de los eventos anteriores.
CREATE TABLE resource_progress (
    enrollment_id      UUID        NOT NULL REFERENCES enrollments(id) ON DELETE CASCADE,
    resource_stable_id UUID        NOT NULL,
    opened_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    dwell_secs         INTEGER     NOT NULL DEFAULT 0,
    last_position_secs INTEGER     NOT NULL DEFAULT 0,
    completed          BOOLEAN     NOT NULL DEFAULT FALSE,
    completed_at       TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (enrollment_id, resource_stable_id)
);

CREATE TABLE badges (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    enrollment_id UUID        NOT NULL REFERENCES enrollments(id) ON DELETE CASCADE,
    course_id     UUID        NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    public_code   TEXT        NOT NULL UNIQUE, -- va en la URL publica, no revela el correo
    image_key     TEXT        NOT NULL DEFAULT '',
    issued_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at    TIMESTAMPTZ,
    revoke_reason TEXT        NOT NULL DEFAULT ''
);
-- Una sola insignia viva por (curso, usuario): garantiza la emision unica
-- aunque la cola entregue el trabajo dos veces.
CREATE UNIQUE INDEX uniq_badge_alive ON badges(course_id, user_id) WHERE revoked_at IS NULL;

-- ---------------------------------------------------------------------
-- 6. IDEMPOTENCIA DE WORKERS
-- ---------------------------------------------------------------------
-- Si la cola reentrega un trabajo ya terminado, el worker lo detecta aqui
-- y no repite el efecto. La llave es la misma que se uso al encolar.
CREATE TABLE processed_jobs (
    job_key      TEXT        PRIMARY KEY,
    task_type    TEXT        NOT NULL,
    result       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------
-- 7. SEMILLA
-- ---------------------------------------------------------------------
-- El administrador inicial NO se crea aqui. Lo crea la API al arrancar
-- (auth.EnsureAdmin) usando ADMIN_EMAIL y ADMIN_PASSWORD del entorno, para
-- que la contrasena se cifre con bcrypt y no quede un hash fijo en el
-- repositorio. Ver cmd/api/main.go.
