"use client";

import { useEffect, useRef, useState } from "react";
import { CheckCircle2, PlayCircle, XCircle } from "lucide-react";

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
}

export function QuizRunner({
  resource,
  onPassed,
}: {
  resource: OutlineResource;
  onPassed: () => void;
}) {
  const [attempt, setAttempt] = useState<AttemptResponse | null>(null);
  const [answers, setAnswers] = useState<Record<string, string[]>>({});
  const [result, setResult] = useState<SubmitResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
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
        setError(data?.error?.message ?? "No se pudo iniciar el intento.");
        return;
      }
      setAttempt(data);
      setAnswers(data.answers ?? {});
      if (data.score !== undefined) {
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
      if (data.passed) onPassed();
    } finally {
      setBusy(false);
    }
  }

  // Pantalla inicial
  if (!attempt) {
    return (
      <Card>
        <CardBody className="space-y-4">
          {error && <Alert variant="error">{error}</Alert>}
          <p className="font-medium">
            Pon a prueba lo aprendido. La calificación la realiza el servidor.
          </p>
          <Button onClick={start} disabled={busy}>
            <PlayCircle className="h-5 w-5" />
            {busy ? "Cargando…" : "Comenzar quiz"}
          </Button>
        </CardBody>
      </Card>
    );
  }

  const finished = result && result.status !== "in_progress";

  return (
    <div className="space-y-4">
      {error && <Alert variant="error">{error}</Alert>}

      {finished && (
        <Alert
          variant={result!.passed ? "success" : "error"}
          title={result!.passed ? "¡Aprobado!" : "No alcanzaste la nota"}
        >
          Puntaje: {Math.round(result!.score)}% (mínimo {attempt.passing_score}%).
          {result!.total_questions > 0 &&
            ` Acertaste ${result!.correct_questions}/${result!.total_questions}.`}
        </Alert>
      )}

      {attempt.questions.map((q, i) => (
        <Card key={q.id}>
          <CardHeader className="flex items-center justify-between gap-2">
            <CardTitle>Pregunta {i + 1}</CardTitle>
            {q.multiple && <Tag tone="purple">Selección múltiple</Tag>}
          </CardHeader>
          <CardBody className="space-y-3">
            <p className="font-bold">{q.prompt}</p>
            <ul className="space-y-2">
              {q.options.map((o) => {
                const selected = (answers[q.id] ?? []).includes(o.id);
                return (
                  <li key={o.id}>
                    <button
                      type="button"
                      disabled={!!finished}
                      onClick={() => toggle(q.id, o.id, q.multiple)}
                      className={`flex w-full items-center gap-3 border-[3px] border-ink px-3 py-2 text-left font-medium brutal-press disabled:cursor-default ${
                        selected ? "bg-accent-yellow" : "bg-[var(--card)]"
                      }`}
                    >
                      <span
                        className={`flex h-5 w-5 shrink-0 items-center justify-center border-2 border-ink ${
                          selected ? "bg-ink text-white" : ""
                        } ${q.multiple ? "" : "rounded-full"}`}
                      >
                        {selected && <CheckCircle2 className="h-3 w-3" />}
                      </span>
                      {o.text}
                    </button>
                  </li>
                );
              })}
            </ul>
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
            idemKey.current = crypto.randomUUID();
          }}
        >
          <XCircle className="h-5 w-5" /> Reintentar
        </Button>
      )}
    </div>
  );
}
