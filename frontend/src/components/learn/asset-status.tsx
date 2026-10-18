"use client";

import { Loader2, ShieldAlert, Clock, XCircle } from "lucide-react";

import { Alert } from "@/components/ui/alert";

/**
 * Traduce el `asset_status` del backend a un estado visible para el estudiante.
 *
 * Ciclo de vida del asset (db/init.sql):
 *   awaiting_upload -> uploaded -> scanning -> (infected)
 *                   -> processing -> ready   (o failed)
 *
 * REQUISITO DE LA ENTREGA: un archivo que aún no está `ready` NO debe mostrarse
 * como disponible. Solo cuando `asset_status === "ready"` se entrega el
 * contenido (el backend también lo exige: GET /assets/:id/url responde 409 si
 * el estado no es `ready`).
 */

export type AssetStatus =
  | "awaiting_upload"
  | "uploaded"
  | "scanning"
  | "infected"
  | "processing"
  | "ready"
  | "failed"
  | "";

/** `true` solo cuando el archivo está listo para entregarse. */
export function isAssetReady(status?: string): boolean {
  return status === "ready";
}

/**
 * Renderiza el estado intermedio de un asset que todavía no está disponible.
 * Devuelve `null` cuando el asset está `ready` (el llamador muestra el
 * contenido real) o cuando no hay asset asociado.
 */
export function AssetStatusNotice({ status }: { status?: string }) {
  switch (status) {
    case "awaiting_upload":
    case "uploaded":
    case "scanning":
      return (
        <Alert variant="info" title="Preparando el archivo" className="items-center">
          <span className="flex items-center gap-2">
            <Loader2 className="h-4 w-4 animate-spin" />
            Estamos verificando el archivo (análisis de seguridad). Vuelve a
            intentarlo en unos momentos.
          </span>
        </Alert>
      );
    case "processing":
      return (
        <Alert variant="warning" title="Procesando" className="items-center">
          <span className="flex items-center gap-2">
            <Loader2 className="h-4 w-4 animate-spin" />
            El contenido se está procesando (transcodificación). Estará
            disponible en breve.
          </span>
        </Alert>
      );
    case "infected":
      return (
        <Alert variant="error" title="Archivo no disponible">
          <span className="flex items-center gap-2">
            <ShieldAlert className="h-4 w-4" />
            El análisis de seguridad rechazó este archivo. Contacta al profesor.
          </span>
        </Alert>
      );
    case "failed":
      return (
        <Alert variant="error" title="Error de procesamiento">
          <span className="flex items-center gap-2">
            <XCircle className="h-4 w-4" />
            No se pudo procesar el archivo. El profesor debe volver a cargarlo.
          </span>
        </Alert>
      );
    case "ready":
    case undefined:
    case "":
      return null;
    default:
      return (
        <Alert variant="info" title="Archivo en preparación" className="items-center">
          <span className="flex items-center gap-2">
            <Clock className="h-4 w-4" />
            El archivo aún no está disponible ({status}).
          </span>
        </Alert>
      );
  }
}
