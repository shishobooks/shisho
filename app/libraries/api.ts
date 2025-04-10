import QueryString from "qs";

export class ShishoAPIError extends Error {
  // The Shisho error code.
  public code: string;
  // The response status code.
  public status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.code = code;
    this.status = status;
    this.name = "ShishoError";
  }
}

class ShishoAPI {
  private uri: string;

  constructor() {
    this.uri = "/api";
  }

  async checkStatus(response: Response) {
    const resp = await response.json();
    if (response.status >= 200 && response.status < 300) {
      return resp;
    }
    throw new ShishoAPIError(
      resp.error.message,
      resp.code || resp.error?.code,
      response.status,
    );
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
}

export const API = new ShishoAPI();
