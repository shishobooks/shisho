# Shisho Docs Website

This folder contains the versioned docs site for Shisho, built with Docusaurus and deployed through GitHub Pages.

## Local development

```bash
cd website
yarn install
yarn start
```

## Production build

```bash
cd website
yarn build
yarn serve
```

## Docs versioning model

- `Unreleased` docs come from `website/docs` on `master`.
- Numbered versions come from `website/versioned_docs`.
- `website/versions.json` tracks available released versions.

Release tagging already snapshots docs automatically by running `yarn docs:version <version>` inside `website/`.
