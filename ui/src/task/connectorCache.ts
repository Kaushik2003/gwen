import { App, wire } from "../api";

const TTL = 60_000;
type Entry<T> = { at: number; promise: Promise<T> };
const lists = new Map<string, Entry<wire.ConnectorList>>();
const searches = new Map<string, Entry<wire.ConnectorItems>>();

function cached<T>(entries: Map<string, Entry<T>>, key: string, request: () => Promise<T>, fresh?: boolean): Promise<T> {
  const hit = entries.get(key);
  if (!fresh && hit && Date.now() - hit.at < TTL) return hit.promise;
  const entry: Entry<T> = {
    at: Date.now(),
    promise: request().catch((error: unknown) => {
      if (entries.get(key) === entry) entries.delete(key);
      throw error;
    }),
  };
  entries.set(key, entry);
  return entry.promise;
}

export function listConnectors(fresh?: boolean): Promise<wire.ConnectorList> {
  const promise = cached(lists, "list", () => App.ListConnectors(), fresh);
  void promise.then((list) => {
    if (list.connectors.every((connector) => connector.status === "unavailable") && lists.get("list")?.promise === promise) lists.delete("list");
  }, () => {});
  return promise;
}

export function searchConnector(app: string, query: string, fresh?: boolean): Promise<wire.ConnectorItems> {
  const trimmed = query.trim();
  const promise = cached(searches, `${app}\u0000${trimmed}`, () => App.SearchConnector(app, trimmed), fresh);
  if (searches.size > 40) {
    const oldest = searches.keys().next();
    if (!oldest.done) searches.delete(oldest.value);
  }
  return promise;
}

/** Forget everything kept: used when an app's sign-in may have changed. */
export function forgetConnectors(): void {
  lists.clear();
  searches.clear();
}
