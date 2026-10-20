import { Navigate, useLocation, useParams } from "react-router-dom";

interface PreservingRedirectProps {
  /** Where the old URL moved. A splat route appends its matched rest. */
  to: string;
  /** The section to open when the old URL names none. */
  hash?: string;
}

// Sends a bookmark of a moved page to its new URL, keeping the query (a
// `?redirect=` back-link) and the hash (a section link).
const PreservingRedirect = ({
  to,
  hash: fallbackHash,
}: PreservingRedirectProps) => {
  const { "*": rest = "" } = useParams();
  const { search, hash } = useLocation();
  const path = rest ? `${to}/${rest}` : to;
  return (
    <Navigate replace to={`${path}${search}${hash || fallbackHash || ""}`} />
  );
};

export default PreservingRedirect;
