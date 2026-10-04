import "server-only";

import { apiFetch } from "./server-client";
import type { Course, CourseVersion, PublishProblem } from "./types";

interface ListWrapper<T> {
  data: T[];
}

export async function listMyCourses(): Promise<Course[]> {
  const res = await apiFetch<ListWrapper<Course>>("/courses");
  return res.data ?? [];
}

export function getCourse(id: string) {
  return apiFetch<Course>(`/courses/${encodeURIComponent(id)}`);
}

export async function listVersions(courseId: string): Promise<CourseVersion[]> {
  const res = await apiFetch<ListWrapper<CourseVersion>>(
    `/courses/${encodeURIComponent(courseId)}/versions`,
  );
  return res.data ?? [];
}

export interface VersionOutline {
  version_id: string;
  status: string;
  modules: import("./types").OutlineModule[];
  publishable: boolean;
  problems: PublishProblem[];
}

/** Previsualizacion del autor: arbol completo + problemas de publicacion. */
export function getVersionOutline(versionId: string) {
  return apiFetch<VersionOutline>(`/versions/${encodeURIComponent(versionId)}`);
}
