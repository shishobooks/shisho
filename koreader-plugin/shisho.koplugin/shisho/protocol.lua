-- The protocol this plugin speaks to a Shisho server. A server refuses a
-- protocol it no longer accepts with 426, which the plugin shows as "update
-- required". Protocol in pkg/koreaderplugin must match.
--
-- capabilities names the features added within this protocol that need the
-- server's support. A server lists the ones it supports on every answer, and
-- the plugin turns each on for a server only when that server lists it
-- (Versions.supports). The server must offer every name here to this
-- protocol, which a Go test checks.
return {
    version = 1,
    capabilities = {
        -- Send reading history again: the server keeps a resent status or
        -- Rating only for an unrecognized copy that holds none.
        resend_history = "resend_history",
    },
}
