// A stand-in for the download endpoint that answers Range requests the way
// Go's http.ServeContent does, for tests of RangeFile. Swap it in with
// vi.spyOn(globalThis, "fetch").mockImplementation(server.fetch).

export interface RangeServerRequest {
  url: string;
  range: string | null;
  ifRange: string | null;
}

export interface RangeServer {
  fetch: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;
  requests: RangeServerRequest[];
  // Body bytes sent in 206 answers.
  bytesSent: () => number;
  // The next `count` requests are answered as a Generated File still being
  // built: a 200 streaming the file, with no Accept-Ranges.
  buildFor: (count: number) => void;
  // Replaces the file, as a rebuild after a metadata edit does.
  replace: (bytes: Uint8Array, lastModified: string) => void;
  // The next `count` requests reject as a dropped connection does.
  failFor: (count: number) => void;
  // How a request for several ranges is answered: in full (as Go's
  // http.ServeContent does), with the whole file (a server that takes one
  // range at a time), with only the first part (a proxy that drops the
  // rest), or with the last part cut short.
  multiRange: (mode: "full" | "whole" | "first" | "cut") => void;
}

export const createRangeServer = (
  initial: Uint8Array,
  lastModified = "Sun, 22 Nov 2026 19:36:13 GMT",
): RangeServer => {
  let bytes = initial;
  let modified = lastModified;
  let building = 0;
  let failing = 0;
  let sent = 0;
  let multiRange: "full" | "whole" | "first" | "cut" = "full";
  const requests: RangeServerRequest[] = [];

  const whole = (acceptRanges: boolean) =>
    new Response(
      // Never closes, as a long build doesn't: a client that reads it whole
      // hangs, so a test catches one that does.
      new ReadableStream<Uint8Array>({
        start(controller) {
          controller.enqueue(bytes.slice(0, 16));
        },
      }),
      {
        status: 200,
        headers: acceptRanges
          ? { "Accept-Ranges": "bytes", "Last-Modified": modified }
          : {},
      },
    );

  return {
    requests,
    bytesSent: () => sent,
    buildFor: (count) => {
      building = count;
    },
    failFor: (count) => {
      failing = count;
    },
    multiRange: (mode) => {
      multiRange = mode;
    },
    replace: (next, nextModified) => {
      bytes = next;
      modified = nextModified;
    },
    fetch: async (input, init) => {
      const headers = new Headers(init?.headers);
      requests.push({
        url: String(input),
        range: headers.get("Range"),
        ifRange: headers.get("If-Range"),
      });
      if (init?.signal?.aborted) {
        throw new DOMException("The operation was aborted.", "AbortError");
      }
      if (failing > 0) {
        failing--;
        throw new TypeError("Failed to fetch");
      }
      if (building > 0) {
        building--;
        return whole(false);
      }
      const range = headers.get("Range");
      const ifRange = headers.get("If-Range");
      // A plain GET downloads the file whole, as the reader does in the
      // background on a desktop.
      if (!range) {
        return new Response(bytes.slice(), {
          status: 200,
          headers: { "Accept-Ranges": "bytes", "Last-Modified": modified },
        });
      }
      if (ifRange !== null && ifRange !== modified) return whole(true);
      const ranges = range
        .replace(/^bytes=/, "")
        .split(",")
        .map((spec) => {
          const match = /^(\d*)-(\d*)$/.exec(spec.trim());
          if (!match) throw new Error(`unexpected Range ${range}`);
          if (match[1] === "") {
            return [
              Math.max(0, bytes.length - Number(match[2])),
              bytes.length - 1,
            ];
          }
          return [
            Number(match[1]),
            match[2] === ""
              ? bytes.length - 1
              : Math.min(Number(match[2]), bytes.length - 1),
          ];
        });
      if (ranges.length > 1) {
        if (multiRange === "whole") {
          return new Response(bytes.slice(), {
            status: 200,
            headers: { "Accept-Ranges": "bytes", "Last-Modified": modified },
          });
        }
        // multipart/byteranges, laid out as Go's mime/multipart writes it
        // for http.ServeContent: the first delimiter starts the body.
        const boundary = "3d6b6a416f9b5";
        const encoder = new TextEncoder();
        const parts: Uint8Array[] = [];
        const sending = multiRange === "first" ? ranges.slice(0, 1) : ranges;
        sending.forEach(([start, end], i) => {
          const cut = multiRange === "cut" && i === sending.length - 1;
          parts.push(
            encoder.encode(
              `${i ? "\r\n" : ""}--${boundary}\r\nContent-Range: bytes ${start}-${end}/${bytes.length}\r\nContent-Type: application/octet-stream\r\n\r\n`,
            ),
            bytes.slice(start, cut ? end - 1 : end + 1),
          );
          sent += end + 1 - start;
        });
        parts.push(encoder.encode(`\r\n--${boundary}--\r\n`));
        const body = new Uint8Array(
          parts.reduce((sum, part) => sum + part.length, 0),
        );
        let at = 0;
        for (const part of parts) {
          body.set(part, at);
          at += part.length;
        }
        return new Response(body, {
          status: 206,
          headers: {
            "Accept-Ranges": "bytes",
            "Content-Type": `multipart/byteranges; boundary=${boundary}`,
            "Last-Modified": modified,
          },
        });
      }
      const [start, end] = ranges[0];
      const body = bytes.slice(start, end + 1);
      sent += body.length;
      return new Response(body, {
        status: 206,
        headers: {
          "Accept-Ranges": "bytes",
          "Content-Range": `bytes ${start}-${end}/${bytes.length}`,
          "Last-Modified": modified,
        },
      });
    },
  };
};
