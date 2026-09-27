import * as App from "./wailsjs/go/main/App";
import { wire } from "./wailsjs/go/models";
import { EventsOn } from "./wailsjs/runtime/runtime";

export { App, wire };

/** An API failure as the host passes it: the wire error body, as JSON. */
export interface ApiError {
  code: string;
  message: string;
  details: Record<string, unknown>;
}

export function apiError(e: unknown): ApiError {
  try {
    const parsed = JSON.parse(String(e));
    if (parsed && typeof parsed.code === "string") return { details: {}, ...parsed };
  } catch {
    // not one of ours
  }
  return { code: "internal", message: String(e), details: {} };
}

/** A rev mismatch: refetch and say so, never retry (docs/08-clients.md#shared-behaviour). */
export function isStale(e: ApiError): boolean {
  return e.code === "conflict" && e.details.field === "rev";
}

/** Subscribes to a daemon event forwarded by the host as "gwen:<name>". */
export function onEvent(name: string, fn: (data: any) => void): () => void {
  return EventsOn(`gwen:${name}`, fn);
}
