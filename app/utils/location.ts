// The hostname the page was opened at. A function of its own so tests can
// stand in another host: jsdom always serves the page from localhost.
export const pageHostname = (): string => window.location.hostname;

// Whether hostname only reaches the machine it's typed on: localhost, a
// 127.x address, ::1, or 0.0.0.0. A URL built from it can't be reached from
// another device, such as an e-reader.
export const isLoopbackHost = (hostname: string): boolean => {
  const host = hostname.toLowerCase().replace(/^\[|\]$/g, "");
  return (
    host === "localhost" ||
    host.endsWith(".localhost") ||
    host === "::1" ||
    host === "0.0.0.0" ||
    /^127(\.\d{1,3}){3}$/.test(host)
  );
};
