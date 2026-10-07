# Demo Mode

`demo_mode` turns the server into the read-only Public Demo. Enforcement is the global middleware in `pkg/server/demo_mode.go`: reads and sign-in pass, every other write gets 403 with no admin bypass, and named download routes are denied. Route families that would expose writes or private data through GETs are left unregistered in `pkg/server/server.go`.

Every route family and download path is allowed, denied, or unregistered. A new one is classified deliberately and covered by a middleware or route-registration test (`TestNew_DemoModeRoutes` is the reference). A new GET or HEAD handler must not make persistent user changes, because the middleware lets every GET through.
