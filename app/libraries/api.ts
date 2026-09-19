import QueryString from "qs";
import { toast } from "sonner";

import type { APIKey, APIKeyShortURL } from "@/types/generated/apikeys";

export class ShishoAPIError extends Error {
  // The Shisho error code.
  public code: string;
  // The response status code.
  public status: number;
  // Set by markErrorDisplayed when a caller renders the error itself.
  public displayed = false;

  constructor(message: string, code: string, status: number) {
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

class ShishoAPI {
  private uri: string;

  constructor() {
    this.uri = "/api";
  }

  async checkStatus(response: Response) {
    // Handle 204 No Content or empty responses
    if (
      response.status === 204 ||
      response.headers.get("content-length") === "0"
    ) {
      if (response.status >= 200 && response.status < 300) {
        return undefined;
      }
    }

    const resp = await response.json();
    if (response.status >= 200 && response.status < 300) {
      return resp;
    }
    const error = new ShishoAPIError(
      resp.error.message,
      resp.code || resp.error?.code,
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
    }).then((response) => this.checkStatus(response));
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
