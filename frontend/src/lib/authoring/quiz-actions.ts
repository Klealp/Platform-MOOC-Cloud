"use server";

import { revalidatePath } from "next/cache";

import { apiFetch, ApiRequestError } from "@/lib/api/server-client";

export type QuizResult = { error?: string; ok?: boolean };

export interface QuizConfigInput {
  max_attempts?: number;
  time_limit_secs?: number;
  passing_score?: number;
  shuffle_questions?: boolean;
  feedback?: "none" | "on_submit" | "after_passing";
}

export async function updateQuizAction(
  quizId: string,
  config: QuizConfigInput,
): Promise<QuizResult> {
  try {
    await apiFetch(`/quizzes/${encodeURIComponent(quizId)}`, {
      method: "PATCH",
      json: config,
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo actualizar el quiz." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}

export interface QuestionOptionInput {
  text: string;
  is_correct: boolean;
  feedback?: string;
}
export interface QuestionInput {
  prompt: string;
  points?: number;
  multiple?: boolean;
  options: QuestionOptionInput[];
}

export async function createQuestionAction(
  quizId: string,
  question: QuestionInput,
): Promise<QuizResult> {
  try {
    await apiFetch(`/quizzes/${encodeURIComponent(quizId)}/questions`, {
      method: "POST",
      json: question,
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo crear la pregunta." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}

export async function updateQuestionAction(
  questionId: string,
  question: QuestionInput,
): Promise<QuizResult> {
  try {
    await apiFetch(`/questions/${encodeURIComponent(questionId)}`, {
      method: "PATCH",
      json: question,
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo actualizar la pregunta." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}

export async function deleteQuestionAction(questionId: string): Promise<QuizResult> {
  try {
    await apiFetch(`/questions/${encodeURIComponent(questionId)}`, {
      method: "DELETE",
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo eliminar la pregunta." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}
