"use client";

import { useRouter } from "next/navigation";
import { useId, useState, useTransition } from "react";
import { Check, Pencil, Plus, Save, Trash2, X } from "lucide-react";

import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input, Select, Field } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { Tag } from "@/components/ui/badge";
import {
  createQuestionAction,
  deleteQuestionAction,
  updateQuestionAction,
  updateQuizAction,
  type QuestionInput,
} from "@/lib/authoring/quiz-actions";

export interface AuthorOption {
  id: string;
  text: string;
  is_correct: boolean;
  feedback: string;
}
export interface AuthorQuestion {
  id: string;
  prompt: string;
  points: number;
  multiple: boolean;
  options: AuthorOption[];
}
export interface AuthorQuiz {
  id: string;
  max_attempts: number;
  time_limit_secs: number;
  passing_score: number;
  shuffle_questions: boolean;
  feedback: "none" | "on_submit" | "after_passing";
  questions: AuthorQuestion[] | null;
}

/**
 * Constructor de quizzes (preguntas de selección múltiple, solo texto).
 * Configura la política del quiz y permite crear, editar y eliminar preguntas
 * con sus opciones y la marca de respuesta correcta. La clave vive solo aquí
 * (vista de autor) y en el servidor; nunca llega al estudiante.
 */
export function QuizBuilder({ quiz }: { quiz: AuthorQuiz }) {
  const router = useRouter();
  const [pending, start] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);

  // El backend serializa una lista vacia como null.
  const questions = quiz.questions ?? [];

  function done(res: { error?: string }, msg?: string) {
    if (res.error) {
      setError(res.error);
      setNotice(null);
      return false;
    }
    setError(null);
    setNotice(msg ?? null);
    router.refresh();
    return true;
  }

  function removeQuestion(q: AuthorQuestion) {
    if (!confirm(`¿Eliminar la pregunta "${q.prompt}"?`)) return;
    start(async () => {
      done(await deleteQuestionAction(q.id), "Pregunta eliminada.");
    });
  }

  return (
    <div className="space-y-6">
      {error && <Alert variant="error">{error}</Alert>}
      {notice && <Alert variant="success">{notice}</Alert>}

      <QuizConfig
        quiz={quiz}
        questions={questions}
        pending={pending}
        onSave={(cfg) =>
          start(async () => {
            done(await updateQuizAction(quiz.id, cfg), "Configuración guardada.");
          })
        }
        onError={setError}
      />

      {/* Preguntas */}
      <section className="space-y-4">
        <h2 className="text-xl font-semibold">Preguntas ({questions.length})</h2>

        {questions.length === 0 && (
          <Alert variant="warning">
            El quiz aún no tiene preguntas. Añade al menos una: un quiz vacío
            no deja publicar el curso.
          </Alert>
        )}

        {questions.map((q, i) =>
          editingId === q.id ? (
            <QuestionForm
              key={q.id}
              title={`Editar pregunta ${i + 1}`}
              initial={q}
              submitLabel="Guardar pregunta"
              onCancel={() => setEditingId(null)}
              onError={setError}
              onSubmit={async (input) => {
                const ok = done(await updateQuestionAction(q.id, input), "Pregunta actualizada.");
                if (ok) setEditingId(null);
                return ok;
              }}
            />
          ) : (
            <Card key={q.id}>
              <CardHeader className="flex items-center justify-between gap-2">
                <CardTitle>
                  {i + 1}. {q.prompt}
                </CardTitle>
                <div className="flex shrink-0 items-center gap-2">
                  <Tag tone={q.multiple ? "purple" : "neutral"}>
                    {q.multiple ? "Varias correctas" : "Una correcta"}
                  </Tag>
                  <button
                    title="Editar"
                    disabled={pending}
                    onClick={() => setEditingId(q.id)}
                    className="rounded-[var(--radius-sm)] border border-[var(--color-border)] p-1.5 transition-colors hover:bg-[var(--color-surface-alt)]"
                  >
                    <Pencil className="h-4 w-4" />
                  </button>
                  <button
                    title="Eliminar"
                    disabled={pending}
                    onClick={() => removeQuestion(q)}
                    className="rounded-[var(--radius-sm)] bg-[var(--color-danger)] p-1.5 text-white transition-colors hover:brightness-95"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </CardHeader>
              <CardBody>
                <ul className="space-y-1">
                  {q.options.map((o) => (
                    <li key={o.id} className="text-sm font-medium">
                      <div className="flex items-center gap-2">
                        {o.is_correct ? (
                          <Check className="h-4 w-4 shrink-0 text-[var(--color-success)]" />
                        ) : (
                          <X className="h-4 w-4 shrink-0 text-[var(--color-text-muted)]" />
                        )}
                        {o.text}
                      </div>
                      {o.feedback && (
                        <p className="ml-6 text-xs font-normal text-[var(--color-text-muted)]">
                          Retroalimentación: {o.feedback}
                        </p>
                      )}
                    </li>
                  ))}
                </ul>
              </CardBody>
            </Card>
          ),
        )}

        <NewQuestion
          count={questions.length}
          onError={setError}
          onSubmit={async (input) =>
            done(await createQuestionAction(quiz.id, input), "Pregunta creada.")
          }
        />
      </section>
    </div>
  );
}

// ------------------------- Configuracion -------------------------

/**
 * El backend guarda la nota minima como porcentaje del puntaje total. Si todas
 * las preguntas valen lo mismo, "acertar k de N" equivale a k/N * 100; se
 * trunca a dos decimales (la columna es NUMERIC(5,2)) para que acertar
 * exactamente k preguntas siempre alcance la nota.
 */
function percentFor(k: number, n: number) {
  return Math.floor((k / n) * 10000) / 100;
}
function minCorrectFor(percent: number, n: number) {
  return Math.ceil((percent / 100) * n - 1e-9);
}

function QuizConfig({
  quiz,
  questions,
  pending,
  onSave,
  onError,
}: {
  quiz: AuthorQuiz;
  questions: AuthorQuestion[];
  pending: boolean;
  onSave: (cfg: {
    passing_score: number;
    max_attempts: number;
    time_limit_secs: number;
    feedback: AuthorQuiz["feedback"];
    shuffle_questions: boolean;
  }) => void;
  onError: (msg: string) => void;
}) {
  const [passing, setPassing] = useState(String(quiz.passing_score));
  const [maxAttempts, setMaxAttempts] = useState(String(quiz.max_attempts));
  const [timeLimit, setTimeLimit] = useState(String(quiz.time_limit_secs));
  const [feedback, setFeedback] = useState(quiz.feedback);
  const [shuffle, setShuffle] = useState(quiz.shuffle_questions);

  const n = questions.length;
  // La equivalencia en preguntas solo es exacta si todas pesan igual.
  const uniform = n > 0 && questions.every((q) => q.points === questions[0].points);
  const passingNum = Number(passing);
  const minCorrect =
    uniform && passing !== "" && passingNum > 0 ? Math.min(minCorrectFor(passingNum, n), n) : "";

  function save() {
    const p = Number(passing);
    const m = Number(maxAttempts);
    const t = Number(timeLimit);
    if (passing === "" || !(p > 0 && p <= 100)) {
      onError("La nota mínima debe estar entre 1 y 100 %.");
      return;
    }
    if (!Number.isInteger(m) || m < 0 || !Number.isInteger(t) || t < 0) {
      onError("Intentos y límite de tiempo deben ser enteros mayores o iguales a 0.");
      return;
    }
    onSave({
      passing_score: p,
      max_attempts: m,
      time_limit_secs: t,
      feedback,
      shuffle_questions: shuffle,
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Configuración</CardTitle>
      </CardHeader>
      <CardBody className="grid gap-4 sm:grid-cols-2">
        <Field label={uniform ? `Respuestas correctas mínimas (de ${n})` : "Respuestas correctas mínimas"}>
          <Input
            type="number"
            min={1}
            max={n || undefined}
            disabled={!uniform}
            value={minCorrect}
            placeholder={n === 0 ? "Añade preguntas primero" : "Preguntas con peso distinto"}
            onChange={(e) => {
              if (e.target.value === "") return;
              const k = Math.max(1, Math.min(n, Math.round(Number(e.target.value))));
              setPassing(String(percentFor(k, n)));
            }}
          />
        </Field>
        <Field label="Nota mínima (%)">
          <Input
            type="number"
            min={1}
            max={100}
            step="0.01"
            value={passing}
            onChange={(e) => setPassing(e.target.value)}
          />
        </Field>
        <Field label="Intentos máx. (0 = ilimitado)">
          <Input
            type="number"
            min={0}
            value={maxAttempts}
            onChange={(e) => setMaxAttempts(e.target.value)}
          />
        </Field>
        <Field label="Límite de tiempo (seg, 0 = sin límite)">
          <Input
            type="number"
            min={0}
            value={timeLimit}
            onChange={(e) => setTimeLimit(e.target.value)}
          />
        </Field>
        <Field label="Retroalimentación">
          <Select
            value={feedback}
            onChange={(e) => setFeedback(e.target.value as AuthorQuiz["feedback"])}
          >
            <option value="none">Ninguna</option>
            <option value="on_submit">Al enviar</option>
            <option value="after_passing">Tras aprobar</option>
          </Select>
        </Field>
        <label className="flex items-center gap-2 self-end pb-2 text-sm font-medium">
          <input
            type="checkbox"
            checked={shuffle}
            onChange={(e) => setShuffle(e.target.checked)}
            className="h-4 w-4 rounded-[var(--radius-sm)] border border-[var(--color-border)]"
          />
          Barajar preguntas
        </label>
        <p className="text-sm text-[var(--color-text-muted)] sm:col-span-2">
          La nota mínima se guarda como porcentaje. Si cambias el número de
          preguntas, revisa este valor y vuelve a guardar.
        </p>
        <div className="sm:col-span-2">
          <Button onClick={save} disabled={pending}>
            <Save className="h-5 w-5" /> Guardar configuración
          </Button>
        </div>
      </CardBody>
    </Card>
  );
}

// ------------------------- Preguntas -------------------------

function NewQuestion({
  count,
  onSubmit,
  onError,
}: {
  count: number;
  onSubmit: (input: QuestionInput) => Promise<boolean>;
  onError: (msg: string) => void;
}) {
  const [open, setOpen] = useState(false);
  if (!open) {
    return (
      <Button variant="outline" onClick={() => setOpen(true)}>
        <Plus className="h-5 w-5" /> Añadir pregunta
      </Button>
    );
  }
  return (
    <QuestionForm
      title={`Nueva pregunta ${count + 1}`}
      submitLabel="Crear pregunta"
      onCancel={() => setOpen(false)}
      onError={onError}
      onSubmit={async (input) => {
        const ok = await onSubmit(input);
        if (ok) setOpen(false);
        return ok;
      }}
    />
  );
}

type DraftOption = { text: string; is_correct: boolean; feedback: string };

function QuestionForm({
  title,
  initial,
  submitLabel,
  onSubmit,
  onCancel,
  onError,
}: {
  title: string;
  initial?: AuthorQuestion;
  submitLabel: string;
  onSubmit: (input: QuestionInput) => Promise<boolean>;
  onCancel: () => void;
  onError: (msg: string) => void;
}) {
  const [prompt, setPrompt] = useState(initial?.prompt ?? "");
  const [multiple, setMultiple] = useState(initial?.multiple ?? false);
  const [options, setOptions] = useState<DraftOption[]>(
    initial?.options.map((o) => ({
      text: o.text,
      is_correct: o.is_correct,
      feedback: o.feedback ?? "",
    })) ?? [
      { text: "", is_correct: true, feedback: "" },
      { text: "", is_correct: false, feedback: "" },
    ],
  );
  const [pending, start] = useTransition();
  const radioGroup = useId();

  function setCorrect(idx: number, checked: boolean) {
    setOptions((prev) =>
      prev.map((o, i) => {
        // Con respuesta unica, marcar una opcion desmarca las demas (el
        // backend rechaza varias correctas si multiple = false).
        if (!multiple) return { ...o, is_correct: i === idx };
        return i === idx ? { ...o, is_correct: checked } : o;
      }),
    );
  }

  function toggleMultiple(value: boolean) {
    setMultiple(value);
    if (!value) {
      // Al volver a respuesta unica se conserva solo la primera correcta.
      setOptions((prev) => {
        const first = prev.findIndex((o) => o.is_correct);
        return prev.map((o, i) => ({ ...o, is_correct: i === (first === -1 ? 0 : first) }));
      });
    }
  }

  function submit() {
    const clean = options
      .map((o) => ({ ...o, text: o.text.trim(), feedback: o.feedback.trim() }))
      .filter((o) => o.text);
    if (!prompt.trim()) {
      onError("Escribe el enunciado de la pregunta.");
      return;
    }
    if (clean.length < 2) {
      onError("La pregunta necesita al menos dos opciones con texto.");
      return;
    }
    const correct = clean.filter((o) => o.is_correct).length;
    if (correct === 0) {
      onError("Marca como correcta al menos una opción que tenga texto.");
      return;
    }
    if (!multiple && correct > 1) {
      onError("Una pregunta de respuesta única solo puede tener una opción correcta.");
      return;
    }
    start(async () => {
      await onSubmit({
        prompt: prompt.trim(),
        points: initial?.points,
        multiple,
        options: clean,
      });
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardBody className="space-y-3">
        <Field label="Enunciado">
          <Input
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="¿Cuál es…?"
          />
        </Field>
        <label className="flex items-center gap-2 text-sm font-medium">
          <input
            type="checkbox"
            checked={multiple}
            onChange={(e) => toggleMultiple(e.target.checked)}
            className="h-4 w-4 rounded-[var(--radius-sm)] border border-[var(--color-border)]"
          />
          Permite varias respuestas correctas
        </label>
        <div className="space-y-2">
          <p className="text-sm text-[var(--color-text-muted)]">
            Marca {multiple ? "las opciones correctas" : "la opción correcta"}. La
            retroalimentación se muestra al estudiante tras enviar, según la
            política elegida en la configuración.
          </p>
          {options.map((o, idx) => (
            <div key={idx} className="space-y-1">
            <div className="flex items-center gap-2">
              <input
                type={multiple ? "checkbox" : "radio"}
                name={radioGroup}
                checked={o.is_correct}
                onChange={(e) => setCorrect(idx, e.target.checked)}
                className="h-5 w-5 shrink-0 border border-[var(--color-border)]"
                title="Correcta"
              />
              <Input
                value={o.text}
                onChange={(e) =>
                  setOptions((prev) =>
                    prev.map((x, i) => (i === idx ? { ...x, text: e.target.value } : x)),
                  )
                }
                placeholder={`Opción ${idx + 1}`}
              />
              {options.length > 2 && (
                <button
                  type="button"
                  title="Quitar opción"
                  onClick={() =>
                    setOptions((prev) => {
                      const next = prev.filter((_, i) => i !== idx);
                      // Si se quita la unica correcta, la primera pasa a serlo.
                      if (!next.some((x) => x.is_correct)) next[0] = { ...next[0], is_correct: true };
                      return next;
                    })
                  }
                  className="rounded-[var(--radius-sm)] bg-[var(--color-danger)] p-1.5 text-white transition-colors hover:brightness-95"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              )}
            </div>
            <Input
              value={o.feedback}
              onChange={(e) =>
                setOptions((prev) =>
                  prev.map((x, i) => (i === idx ? { ...x, feedback: e.target.value } : x)),
                )
              }
              placeholder="Retroalimentación al elegir esta opción (opcional)"
              className="ml-7 w-[calc(100%-1.75rem)] text-sm"
            />
            </div>
          ))}
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setOptions((prev) => [...prev, { text: "", is_correct: false, feedback: "" }])}
          >
            <Plus className="h-4 w-4" /> Opción
          </Button>
        </div>
        <div className="flex gap-2">
          <Button size="sm" onClick={submit} disabled={pending}>
            {pending ? "Guardando…" : submitLabel}
          </Button>
          <Button size="sm" variant="ghost" onClick={onCancel} disabled={pending}>
            Cancelar
          </Button>
        </div>
      </CardBody>
    </Card>
  );
}
