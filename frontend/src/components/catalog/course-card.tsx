import Link from "next/link";
import { Users } from "lucide-react";

import { Card, CardBody, CardFooter } from "@/components/ui/card";
import { Tag } from "@/components/ui/badge";
import type { CatalogItem } from "@/lib/api/types";

const levelTone = {
  beginner: "ok",
  intermediate: "yellow",
  advanced: "pink",
} as const;

export function CourseCard({ course }: { course: CatalogItem }) {
  const tone =
    levelTone[course.level as keyof typeof levelTone] ?? "neutral";
  return (
    <Link href={`/catalog/${course.slug}`} className="block">
      <Card interactive className="flex h-full flex-col">
        <CardBody className="flex-1 space-y-3">
          <div className="flex flex-wrap gap-2">
            {course.category && <Tag tone="blue">{course.category}</Tag>}
            {course.level && <Tag tone={tone}>{course.level}</Tag>}
          </div>
          <h3 className="text-xl font-extrabold uppercase leading-tight">
            {course.title}
          </h3>
          <p className="line-clamp-3 text-sm font-medium opacity-80">
            {course.summary || "Sin descripción."}
          </p>
          {course.tags?.length > 0 && (
            <div className="flex flex-wrap gap-1">
              {course.tags.slice(0, 4).map((t) => (
                <span key={t} className="text-xs font-bold opacity-60">
                  #{t}
                </span>
              ))}
            </div>
          )}
        </CardBody>
        <CardFooter className="flex items-center justify-between text-sm font-bold">
          <span className="truncate">{course.teacher}</span>
          <span className="flex items-center gap-1">
            <Users className="h-4 w-4" /> {course.enrolled}
          </span>
        </CardFooter>
      </Card>
    </Link>
  );
}
