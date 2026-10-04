/**
 * Tipos de dominio del frontend.
 *
 * El `openapi.yaml` del backend describe muy bien los cuerpos de ENTRADA
 * (request bodies), pero deja casi todas las RESPUESTAS como simples
 * descripciones sin esquema. Por eso aqui tipamos a mano las formas de
 * respuesta reales — tomadas de los handlers Go (gin.H{...}) — para tener un
 * cliente seguro. Los tipos de request se derivan de schema.d.ts cuando aplica.
 */

export type Role = "student" | "teacher" | "admin";
export type CourseLevel = "beginner" | "intermediate" | "advanced";
export type ResourceType =
  | "rich_text"
  | "image"
  | "video"
  | "audio"
  | "pdf"
  | "download"
  | "external_link"
  | "quiz";

/** Forma estandar de error del backend (schema `Error`). */
export interface ApiError {
  error: {
    code:
      | "validation_error"
      | "unauthorized"
      | "forbidden"
      | "not_found"
      | "conflict"
      | "rate_limited"
      | "unprocessable"
      | "payload_too_large"
      | "internal_error";
    message: string;
    details?: unknown;
    request_id?: string;
  };
}

// ------------------------- Auth / sesion -------------------------

export interface SessionUser {
  id: string;
  email: string;
  full_name: string;
  role: Role;
  status?: string;
  session_id?: string;
}

export interface LoginResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  session_id: string;
  user: SessionUser;
}

export interface ActiveSession {
  id: string;
  user_agent: string;
  ip: string;
  created_at: string;
  last_seen_at: string;
  expires_at: string;
  current: boolean;
}

// ------------------------- Catalogo -------------------------

export interface CatalogItem {
  id: string;
  slug: string;
  title: string;
  summary: string;
  category: string;
  level: CourseLevel | string;
  language: string;
  tags: string[];
  teacher: string;
  published_at: string | null;
  enrolled: number;
}

export interface CatalogListResponse {
  data: CatalogItem[];
  next_cursor: string;
}

export interface OutlineResource {
  id: string;
  stable_id: string;
  type: ResourceType;
  title: string;
  position: number;
  visible: boolean;
  required: boolean;
  downloadable: boolean;
  processing_status: string;
  asset_id?: string;
  asset_status?: string;
  quiz_id?: string;
  external_url?: string;
  has_content: boolean;
}

export interface OutlineUnit {
  id: string;
  stable_id: string;
  title: string;
  position: number;
  resources: OutlineResource[];
}

export interface OutlineModule {
  id: string;
  stable_id: string;
  title: string;
  position: number;
  units: OutlineUnit[];
}

export interface CatalogDetail {
  id: string;
  slug: string;
  title: string;
  summary: string;
  category: string;
  level: string;
  language: string;
  tags: string[];
  teacher: string;
  version_id: string;
  published_at: string | null;
  passing_score: number;
  required_completion: number;
  outline: OutlineModule[];
}

// ------------------------- Aprendizaje / progreso -------------------------

export interface ProgressResult {
  progress_pct: number;
  state: string;
  completed_required: number;
  total_required: number;
  passing: boolean;
  approved: boolean;
  newly_approved?: boolean;
}

export interface Enrollment {
  id: string;
  course_id: string;
  slug: string;
  title: string;
  status: string;
  state: string;
  progress_pct: number;
  enrolled_at: string;
  completed_at: string | null;
  approved_at: string | null;
}

export interface ResourceProgress {
  completed: boolean;
  dwell_secs: number;
  last_position_secs: number;
}

export interface EnrollmentOutline {
  enrollment_id: string;
  course_id: string;
  version_id: string;
  modules: OutlineModule[];
  resource_progress: Record<string, ResourceProgress>;
  progress: ProgressResult;
}

export type ProgressEventType = "open" | "heartbeat" | "complete";

export interface ProgressEventBody {
  enrollment_id: string;
  resource_stable_id: string;
  event_type: ProgressEventType;
  position_secs?: number;
  delta_secs?: number;
}

// ------------------------- Quizzes -------------------------

export interface QuizOptionView {
  id: string;
  text: string;
}

export interface QuizQuestionView {
  id: string;
  prompt: string;
  points: number;
  multiple: boolean;
  options: QuizOptionView[];
}

export interface AttemptView {
  id: string;
  quiz_id: string;
  status: string;
  started_at: string;
  expires_at?: string | null;
  questions: QuizQuestionView[];
  answers?: Record<string, string[]>;
}

export interface AttemptResult {
  id: string;
  status: string;
  score: number;
  passing_score: number;
  passed: boolean;
  submitted_at: string;
}

// ------------------------- Insignias -------------------------

export interface Badge {
  id: string;
  code: string;
  course_title: string;
  issued_at: string;
  revoked_at: string | null;
  verify_url: string;
}

export interface PublicBadge {
  valid: boolean;
  revoked: boolean;
  student_name: string;
  course_title: string;
  issued_at: string;
}

// ------------------------- Autoria -------------------------

export interface Course {
  id: string;
  slug: string;
  title: string;
  summary?: string;
  status: string;
  category?: string;
  level?: string;
  language?: string;
  tags?: string[];
  current_version_id?: string;
  updated_at?: string;
  active_enrollments?: number;
}

export interface CourseVersion {
  id: string;
  version_number: number;
  status: string;
  passing_score: number;
  required_completion: number;
  published_at: string | null;
  created_at: string;
}

export interface PublishProblem {
  code: string;
  path: string;
  message: string;
}

export interface ResourceContent {
  content_md: string;
  revision: number;
}

// ------------------------- Admin -------------------------

export interface AdminUser {
  id: string;
  email: string;
  full_name: string;
  role: Role;
  status: string;
  email_verified: boolean;
  created_at: string;
}

export interface AuditEntry {
  id: number;
  actor_id: string;
  action: string;
  entity_type: string;
  entity_id: string;
  ip: string;
  metadata: Record<string, unknown>;
  created_at: string;
}
