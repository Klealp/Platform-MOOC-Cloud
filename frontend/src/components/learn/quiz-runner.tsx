"use client";

import { useEffect, useRef, useState } from "react";
import { PlayCircle, XCircle } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert } from "@/components/ui/alert";
import { Tag } from "@/components/ui/badge";
import type { AttemptView, OutlineResource } from "@/lib/api/types";

/**
 * Presentacion de un quiz. Flujo:
 *  1. Inicia un intento (POST /quizzes/:id/attempts). El backend devuelve las
 *     preguntas SIN la respuesta correcta (la clave nunca llega al cliente).
 *  2. El estudiante selecciona opciones; se guardan parcialmente (PATCH).
 *  3. Al enviar (POST .../submit) el backend califica y responde el puntaje.
 *     La calificacion es siempre del servidor; aqui solo mostramos el resultado.
 */

interface AttemptResponse extends AttemptView {
  passing_score: number;
  attempt_number?: number;
  seconds_left?: number;
  score?: number;
  passed?: boolean;
}

interface SubmitResponse {
  status: string;
  score: number;
  passed: boolean;
  correct_questions: number;
  total_questions: number;
  questions?: AttemptView["questions"];
  progress?: { progress_pct: number; state: string };
}

export function QuizRunner({
  resource,
  onPassed,
}: {
  resource: OutlineResource;
  /** Recibe el progreso del curso recalculado por el servidor al aprobar. */
  onPassed: (progress?: { progress_pct: number; state: string }) => void;
}) {
  const [attempt, setAttempt] = useState<AttemptResponse | null>(null);
  const [answers, setAnswers] = useState<Record<string, string[]>>({});
  const [result, setResult] = useState<SubmitResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [exhausted, setExhausted] = useState<{ max: number; used: number } | null>(
    null,
  );
  const idemKey = useRef<string>(crypto.randomUUID());

  if (!resource.quiz_id) {
    return <Alert variant="warning">Este quiz aún no está configurado.</Alert>;
  }
  const quizId = resource.quiz_id;

  async function start() {
    setError(null);
    setBusy(true);
    try {
      const res = await fetch(`/api/quizzes/${quizId}/attempts`, { method: "POST" });
      const data = await res.json();
      if (!res.ok) {
        // 409 = intentos agotados: el backend informa max_attempts y used.
        if (res.status === 409 && data?.error?.details) {
          const d = data.error.details as { max_attempts?: number; used?: number };
          setExhausted({ max: d.max_attempts ?? 0, used: d.used ?? 0 });
        }
        setError(data?.error?.message ?? "No se pudo iniciar el intento.");
        return;
      }
      setAttempt(data);
      setAnswers(data.answers ?? {});
      // Si el backend devolvió un intento YA enviado (p. ej. recargaste tras
      // enviar), mostramos directamente su resultado.
      if (data.score !== undefined && data.status !== "in_progress") {
        setResult({
          status: data.status,
          score: data.score,
          passed: data.passed,
          correct_questions: 0,
          total_questions: data.questions?.length ?? 0,
        });
      }
    } finally {
      setBusy(false);
    }
  }

  function toggle(questionId: string, optionId: string, multiple: boolean) {
    setAnswers((prev) => {
      const current = prev[questionId] ?? [];
      if (multiple) {
        return {
          ...prev,
          [questionId]: current.includes(optionId)
            ? current.filter((o) => o !== optionId)
            : [...current, optionId],
        };
      }
      return { ...prev, [questionId]: [optionId] };
    });
  }

  async function submit() {
    if (!attempt) return;
    setBusy(true);
    setError(null);
    try {
      const res = await fetch(`/api/attempts/${attempt.id}/submit`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ answers, idempotency_key: idemKey.current }),
      });
      const data = await res.json();
      if (!res.ok) {
        setError(data?.error?.message ?? "No se pudo enviar el intento.");
        return;
      }
      setResult(data);
      // Si la politica del quiz lo permite, el backend devuelve las preguntas
      // con la retroalimentacion de cada opcion; se reemplazan para mostrarla.
      if (Array.isArray(data.questions)) {
        setAttempt((prev) => (prev ? { ...prev, questions: data.questions } : prev));
      }
      if (data.passed) onPassed(data.progress);
    } finally {
      setBusy(false);
    }
  }

  // Pantalla inicial
  if (!attempt) {
    return (
      <Card>
        <CardBody className="space-y-4">
          {exhausted ? (
            <Alert variant="warning" title="Sin intentos disponibles">
              Agotaste los intentos de este quiz ({exhausted.used} de{" "}
              {exhausted.max}).
            </Alert>
          ) : (
            error && <Alert variant="error">{error}</Alert>
          )}
          {!exhausted && (
            <>
              <p className="font-medium">
                Pon a prueba lo aprendido. La calificación la realiza el
                servidor.
              </p>
              <Button onClick={start} disabled={busy}>
                <PlayCircle className="h-5 w-5" />
                {busy ? "Cargando…" : "Comenzar quiz"}
              </Button>
            </>
          )}
        </CardBody>
      </Card>
    );
  }

  const finished = result && result.status !== "in_progress";

  return (
    <div className="space-y-4">
      {error && <Alert variant="error">{error}</Alert>}

      <div className="flex flex-wrap items-center gap-2">
        {attempt.attempt_number ? (
          <Tag tone="neutral">Intento #{attempt.attempt_number}</Tag>
        ) : null}
        <Tag tone="blue">Nota mínima {attempt.passing_score}%</Tag>
        {attempt.status === "in_progress" && !finished && (
          <Tag tone="warn">En curso</Tag>
        )}
      </div>

      {finished && (
        <Alert
          variant={result!.passed ? "success" : "error"}
          title={result!.passed ? "¡Aprobado!" : "No alcanzaste la nota"}
        >
          Puntaje: {Math.round(result!.score)}% (mínimo {attempt.passing_score}%).
          {result!.total_questions > 0 &&
            ` Acertaste ${result!.correct_questions}/${result!.total_questions}.`}
          {result!.status === "expired" && " El tiempo del intento expiró."}
        </Alert>
      )}

      {attempt.questions.map((q, i) => (
        <Card key={q.stable_id}>
          <CardHeader className="flex items-center justify-between gap-2">
            <CardTitle>Pregunta {i + 1}</CardTitle>
            {q.multiple && <Tag tone="purple">Selección múltiple</Tag>}
          </CardHeader>
          <CardBody>
            {/* Opciones nativas (radio o checkbox) en un fieldset cuyo legend es
                el enunciado: el lector de pantalla anuncia la pregunta, el tipo
                de control y si cada opcion esta marcada (WCAG 1.3.1 y 4.1.2). */}
            <fieldset className="space-y-3" disabled={!!finished}>
              <legend className="mb-3 font-bold">{q.prompt}</legend>
              <ul className="space-y-2">
                {q.options.map((o) => {
                  const selected = (answers[q.stable_id] ?? []).includes(o.stable_id);
                  const feedbackId = `fb-${q.stable_id}-${o.stable_id}`;
                  const showFeedback = finished && selected && o.feedback;
                  return (
                    <li key={o.stable_id}>
                      <label
                        className={`flex w-full cursor-pointer items-center gap-3 rounded-[var(--radius-sm)] border px-3 py-2 text-left font-medium transition-colors has-[:disabled]:cursor-default ${
                          selected
                            ? "border-[var(--color-primary)] bg-[color-mix(in_srgb,var(--color-primary)_10%,transparent)]"
                            : "border-[var(--color-border-strong)] bg-[var(--color-surface)]"
                        }`}
                      >
                        <input
                          type={q.multiple ? "checkbox" : "radio"}
                          name={`pregunta-${q.stable_id}`}
                          checked={selected}
                          onChange={() => toggle(q.stable_id, o.stable_id, q.multiple)}
                          aria-describedby={showFeedback ? feedbackId : undefined}
                          className="h-5 w-5 shrink-0 accent-[var(--color-primary)]"
                        />
                        {o.text}
                      </label>
                      {showFeedback && (
                        <p id={feedbackId} className="mt-1 ml-8 text-sm text-[var(--color-text-muted)]">
                          {o.feedback}
                        </p>
                      )}
                    </li>
                  );
                })}
              </ul>
            </fieldset>
          </CardBody>
        </Card>
      ))}

      {!finished && (
        <Button size="lg" onClick={submit} disabled={busy}>
          {busy ? "Enviando…" : "Enviar respuestas"}
        </Button>
      )}
      {finished && !result!.passed && (
        <Button
          variant="secondary"
          onClick={() => {
            setAttempt(null);
            setResult(null);
            setAnswers({});
            setError(null);
            setExhausted(null);
            idemKey.current = crypto.randomUUID();
          }}
        >
          <XCircle className="h-5 w-5" /> Reintentar
        </Button>
      )}
    </div>
  );
}
