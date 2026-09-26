import QueryString from "qs";
import { toast } from "sonner";

import type { APIKey, APIKeyShortURL } from "@/types/generated/apikeys";

export class ShishoAPIError extends Error {
  // The Shisho error code. Undefined when the response did not carry a
  // Shisho error body (e.g. a reverse proxy's HTML 502/504 page).
  public code: string | undefined;
  // The response status code.
  public status: number;
  // Set by markErrorDisplayed when a caller renders the error itself.
  public displayed = false;

  constructor(message: string, code: string | undefined, status: number) {
    super(message);
    this.code = code;
    this.status = status;
    this.name = "ShishoError";
  }
}

const DEMO_MODE_MESSAGE = "This action is unavailable in the demo.";

// Call from a catch block that renders the error inline (e.g. a dialog error
// banner) so the global Demo Mode toast does not repeat the same message.
export const markErrorDisplayed = (error: unknown) => {
  if (error instanceof ShishoAPIError) {
    error.displayed = true;
  }
};

const scheduleDemoModeToast = (error: ShishoAPIError) => {
  // Caller-level error handlers run before the next task. Give them a chance
  // to show the same message, then provide the global fallback only if needed.
  setTimeout(() => {
    if (error.displayed) return;
    // Callers often toast the server message themselves, sometimes with a
    // prefix ("Failed to save: ..."). getToasts() only returns active toasts.
    const alreadyVisible = toast
      .getToasts()
      .some(
        (item) =>
          "title" in item &&
          typeof item.title === "string" &&
          item.title.includes(error.message),
      );
    if (!alreadyVisible) {
      toast.error(DEMO_MODE_MESSAGE, { id: "demo-mode" });
    }
  }, 0);
};

// Reads the { error: { code, message } } body the Go API sends on failure.
// Fields are undefined for any other shape so the caller can fall back to a
// status-based message.
const shishoErrorBody = (
  body: unknown,
): { code?: string; message?: string } => {
  if (typeof body !== "object" || body === null || !("error" in body)) {
    return {};
  }
  const { error } = body as { error: unknown };
  if (typeof error !== "object" || error === null) return {};
  const { code, message } = error as { code?: unknown; message?: unknown };
  return {
    code: typeof code === "string" && code !== "" ? code : undefined,
    message:
      typeof message === "string" && message !== "" ? message : undefined,
  };
};

const statusMessage = (response: Response) =>
  response.statusText
    ? `Request failed with status ${response.status} (${response.statusText})`
    : `Request failed with status ${response.status}`;

class ShishoAPI {
  private uri: string;

  constructor() {
    this.uri = "/api";
  }

  // Every API endpoint answers with JSON or an empty body. Anything else came
  // from something in front of the Go server (a reverse proxy's 502/504 page,
  // a load balancer's 413), so it becomes a ShishoAPIError instead of a JSON
  // SyntaxError:
  // - 2xx with an empty body (including 204) resolves to undefined.
  // - 2xx with a non-JSON body rejects: the caller expected data and would
  //   otherwise get a string it cannot use.
  // - non-2xx without a Shisho error body rejects with a status-based message
  //   and an undefined code.
  async checkStatus<T = unknown>(response: Response): Promise<T> {
    // Read the body as text rather than trusting content-length or
    // content-type: chunked empty bodies have no content-length, and proxies
    // label their error pages inconsistently.
    const text = await response.text();
    let body: unknown;
    if (text.trim() !== "") {
      try {
        body = JSON.parse(text);
      } catch {
        if (response.ok) {
          throw new ShishoAPIError(
            `Received a non-JSON response with status ${response.status}`,
            undefined,
            response.status,
          );
        }
        // Leave body undefined so the error below uses the status message.
      }
    }

    if (response.ok) {
      // Callers type the response; an empty body resolves to undefined.
      return body as T;
    }

    const { code, message } = shishoErrorBody(body);
    const error = new ShishoAPIError(
      message ?? statusMessage(response),
      code,
      response.status,
    );
    if (error.status === 403 && error.code === "demo_mode") {
      scheduleDemoModeToast(error);
    }
    throw error;
  }

  request<T, U = unknown, V = unknown>(
    method: string,
    endpoint: string,
    payload?: U,
    query?: V,
    signal?: AbortSignal,
  ): Promise<T> {
    const headers: Record<string, string> = {
      "content-type": "application/json; charset=utf-8",
      "X-Version": __APP_VERSION__,
    };

    let body = undefined;
    if (payload) {
      body = JSON.stringify(payload);
    }

    let uri = `${this.uri}${endpoint}`;
    if (query) {
      const queryString = QueryString.stringify(query, { indices: false });
      if (queryString) {
        uri = `${uri}?${queryString}`;
      }
    }

    return fetch(uri, {
      method,
      headers,
      body,
      signal,
    }).then((response) => this.checkStatus<T>(response));
  }

  // API Key methods
  listApiKeys(): Promise<APIKey[]> {
    return this.request("GET", "/user/api-keys");
  }

  createApiKey(name: string): Promise<APIKey> {
    return this.request("POST", "/user/api-keys", { name });
  }

  updateApiKeyName(id: string, name: string): Promise<APIKey> {
    return this.request("PATCH", `/user/api-keys/${id}`, { name });
  }

  deleteApiKey(id: string): Promise<void> {
    return this.request("DELETE", `/user/api-keys/${id}`);
  }

  addApiKeyPermission(id: string, permission: string): Promise<APIKey> {
    return this.request(
      "POST",
      `/user/api-keys/${id}/permissions/${permission}`,
    );
  }

  removeApiKeyPermission(id: string, permission: string): Promise<APIKey> {
    return this.request(
      "DELETE",
      `/user/api-keys/${id}/permissions/${permission}`,
    );
  }

  generateApiKeyShortUrl(id: string): Promise<APIKeyShortURL> {
    return this.request("POST", `/user/api-keys/${id}/short-url`);
  }

  clearKoboSync(id: string): Promise<void> {
    return this.request("DELETE", `/user/api-keys/${id}/kobo-sync`);
  }
}

export const API = new ShishoAPI();
