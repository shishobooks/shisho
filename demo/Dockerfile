# Public Demo image: a pinned Shisho release plus the Demo Corpus baked in.
# Built by .github/workflows/demo.yml with the corpus checked out at demo/corpus.
# Contains no application changes; see demo/README.md.
ARG SHISHO_VERSION
FROM ghcr.io/shishobooks/shisho:${SHISHO_VERSION}

COPY --chown=1000:1000 corpus/library /media
COPY --chown=1000:1000 corpus/config/shisho.db /config/shisho.db

ENV DEMO_MODE=true \
    SYNC_INTERVAL_MINUTES=0 \
    LIBRARY_MONITOR_ENABLED=false
