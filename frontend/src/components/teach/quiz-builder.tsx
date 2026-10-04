"use client";

import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import { Check, Plus, Save, Trash2, X } from "lucide-react";

import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input, Select, Field } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { Tag } from "@/components/ui/badge";
import {
  createQuestionAction,
  deleteQuestionAction,
  updateQuizAction,
  type QuestionInput,
} from "@/lib/authoring/quiz-actions";

export interface AuthorOption {
  id: string;
  text: string;
  is_correct: boolean;
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
  questions: AuthorQuestion[];
}

/**
 * Constructor de quizzes (preguntas de selección múltiple, solo texto).
 * Configura la política del quiz y permite crear/eliminar preguntas con sus
 * opciones y la marca de respuesta correcta. La clave vive solo aquí (vista de
 * autor) y en el servidor; nunca llega al estudiante.
 */
export function QuizBuilder({ quiz }: { quiz: AuthorQuiz }) {
  const router = useRouter();
  const [pending, start] = useTransition();
  const [error, setError] = useState<string | null>(null);

  // Config
  const [passing, setPassing] = useState(quiz.passing_score);
  const [maxAttempts, setMaxAttempts] = useState(quiz.max_attempts);
  const [timeLimit, setTimeLimit] = useState(quiz.time_limit_secs);
  const [feedback, setFeedback] = useState(quiz.feedback);
  const [shuffle, setShuffle] = useState(quiz.shuffle_questions);

  function saveConfig() {
    setError(null);
    start(async () => {
      const res = await updateQuizAction(quiz.id, {
        passing_score: passing,
        max_attempts: maxAttempts,
        time_limit_secs: timeLimit,
        feedback,
        shuffle_questions: shuffle,
      });
      if (res.error) setError(res.error);
      else router.refresh();
    });
  }

  function removeQuestion(id: string) {
    start(async () => {
      const res = await deleteQuestionAction(id);
      if (res.error) setError(res.error);
      else router.refresh();
    });
  }

  return (
    <div className="space-y-6">
      {error && <Alert variant="error">{error}</Alert>}

      {/* Configuración */}
      <Card>
        <CardHeader>
          <CardTitle>Configuración</CardTitle>
        </CardHeader>
        <CardBody className="grid gap-4 sm:grid-cols-2">
          <Field label="Nota mínima (%)">
            <Input type="number" min={1} max={100} value={passing}
              onChange={(e) => setPassing(Number(e.target.value))} />
          </Field>
          <Field label="Intentos máx. (0 = ilimitado)">
            <Input type="number" min={0} value={maxAttempts}
              onChange={(e) => setMaxAttempts(Number(e.target.value))} />
          </Field>
          <Field label="Límite de tiempo (seg, 0 = sin límite)">
            <Input type="number" min={0} value={timeLimit}
              onChange={(e) => setTimeLimit(Number(e.target.value))} />
          </Field>
          <Field label="Retroalimentación">
            <Select value={feedback} onChange={(e) => setFeedback(e.target.value as AuthorQuiz["feedback"])}>
              <option value="none">Ninguna</option>
              <option value="on_submit">Al enviar</option>
              <option value="after_passing">Tras aprobar</option>
            </Select>
          </Field>
          <label className="flex items-center gap-2 text-sm font-bold">
            <input type="checkbox" checked={shuffle} onChange={(e) => setShuffle(e.target.checked)}
              className="h-4 w-4 border-2 border-ink" />
            Barajar preguntas
          </label>
          <div className="sm:col-span-2">
            <Button onClick={saveConfig} disabled={pending}>
              <Save className="h-5 w-5" /> Guardar configuración
            </Button>
          </div>
        </CardBody>
      </Card>

      {/* Preguntas */}
      <section className="space-y-4">
        <h2 className="text-xl font-extrabold uppercase">
          Preguntas ({quiz.questions.length})
        </h2>
        {quiz.questions.map((q, i) => (
          <Card key={q.id}>
            <CardHeader className="flex items-center justify-between gap-2">
              <CardTitle>
                {i + 1}. {q.prompt}
              </CardTitle>
              <div className="flex items-center gap-2">
                {q.multiple && <Tag tone="purple">múltiple</Tag>}
                <button
                  title="Eliminar"
                  disabled={pending}
                  onClick={() => removeQuestion(q.id)}
                  className="border-2 border-ink bg-danger p-1.5 text-white brutal-press"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            </CardHeader>
            <CardBody>
              <ul className="space-y-1">
                {q.options.map((o) => (
                  <li key={o.id} className="flex items-center gap-2 text-sm font-medium">
                    {o.is_correct ? (
                      <Check className="h-4 w-4 text-ok" />
                    ) : (
                      <X className="h-4 w-4 opacity-30" />
                    )}
                    {o.text}
                  </li>
                ))}
              </ul>
            </CardBody>
          </Card>
        ))}

        <NewQuestionForm
          quizId={quiz.id}
          onCreated={() => router.refresh()}
          onError={setError}
        />
      </section>
    </div>
  );
}

function NewQuestionForm({
  quizId,
  onCreated,
  onError,
}: {
  quizId: string;
  onCreated: () => void;
  onError: (msg: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [prompt, setPrompt] = useState("");
  const [multiple, setMultiple] = useState(false);
  const [options, setOptions] = useState<{ text: string; is_correct: boolean }[]>([
    { text: "", is_correct: true },
    { text: "", is_correct: false },
  ]);
  const [pending, start] = useTransition();

  function reset() {
    setPrompt("");
    setMultiple(false);
    setOptions([
      { text: "", is_correct: true },
      { text: "", is_correct: false },
    ]);
  }

  function submit() {
    const clean = options.filter((o) => o.text.trim());
    if (!prompt.trim() || clean.length < 2) {
      onError("Necesitas un enunciado y al menos dos opciones.");
      return;
    }
    if (!clean.some((o) => o.is_correct)) {
      onError("Marca al menos una opción correcta.");
      return;
    }
    const input: QuestionInput = {
      prompt: prompt.trim(),
      multiple,
      options: clean.map((o) => ({ text: o.text.trim(), is_correct: o.is_correct })),
    };
    start(async () => {
      const res = await createQuestionAction(quizId, input);
      if (res.error) onError(res.error);
      else {
        reset();
        setOpen(false);
        onCreated();
      }
    });
  }

  if (!open) {
    return (
      <Button variant="outline" onClick={() => setOpen(true)}>
        <Plus className="h-5 w-5" /> Añadir pregunta
      </Button>
    );
  }

  return (
    <Card>
      <CardBody className="space-y-3">
        <Field label="Enunciado">
          <Input value={prompt} onChange={(e) => setPrompt(e.target.value)} placeholder="¿Pregunta?" />
        </Field>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input type="checkbox" checked={multiple} onChange={(e) => setMultiple(e.target.checked)}
            className="h-4 w-4 border-2 border-ink" />
          Permite varias respuestas correctas
        </label>
        <div className="space-y-2">
          {options.map((o, idx) => (
            <div key={idx} className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={o.is_correct}
                onChange={(e) =>
                  setOptions((prev) =>
                    prev.map((x, i) => (i === idx ? { ...x, is_correct: e.target.checked } : x)),
                  )
                }
                className="h-5 w-5 border-2 border-ink"
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
                  onClick={() => setOptions((prev) => prev.filter((_, i) => i !== idx))}
                  className="border-2 border-ink bg-danger p-1.5 text-white brutal-press"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              )}
            </div>
          ))}
          <Button
            size="sm"
            variant="ghost"
            onClick={() => setOptions((prev) => [...prev, { text: "", is_correct: false }])}
          >
            <Plus className="h-4 w-4" /> Opción
          </Button>
        </div>
        <div className="flex gap-2">
          <Button size="sm" onClick={submit} disabled={pending}>
            {pending ? "Creando…" : "Crear pregunta"}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => { reset(); setOpen(false); }}>
            Cancelar
          </Button>
        </div>
      </CardBody>
    </Card>
  );
}
