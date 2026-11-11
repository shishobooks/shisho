-- The protocol this plugin speaks to a Shisho server. A server refuses a
-- protocol it no longer accepts with 426, which the plugin shows as "update
-- required". Protocol in pkg/koreaderplugin must match.
return {
    version = 1,
}
