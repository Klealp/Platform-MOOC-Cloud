import "server-only";

import { apiFetch } from "./server-client";
import type {
  Badge,
  Enrollment,
  EnrollmentOutline,
} from "./types";

interface ListWrapper<T> {
  data: T[];
}

export async function listEnrollments(): Promise<Enrollment[]> {
  const res = await apiFetch<ListWrapper<Enrollment>>("/enrollments");
  return res.data ?? [];
}

export function getEnrollmentOutline(id: string) {
  return apiFetch<EnrollmentOutline>(
    `/enrollments/${encodeURIComponent(id)}/outline`,
  );
}

export async function listMyBadges(): Promise<Badge[]> {
  const res = await apiFetch<ListWrapper<Badge>>("/badges");
  return res.data ?? [];
}
