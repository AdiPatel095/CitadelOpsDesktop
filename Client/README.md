[Desktop visual snapshot checks](Docs/VisualSnapshots.md) cover the fixture dashboard on the studio Mac reference renderer.

## Desktop checks

Run from `Client/` before handing a PR to QA:

```sh
npm run lint:gate
npm run lint
npm run typecheck:app
npm test
npm run build
/Users/nebulabot/Desktop/CIT/tools/visual-lock.sh desktop npm run test:visual
```

`lint:gate` is the blocking ESLint check for CIT-104 Step 1. It rejects every
non-compiler-rule error and every increase per compiler rule and file against
`lint-baseline.json` (48 existing errors). Warnings do not block. `lint` runs the
full unchanged config and currently exits 1 for those remaining hooks errors;
they are tracked in CIT-134. Do not regenerate the baseline to accept new errors
or update visual baselines for this mechanical cleanup. Run `go test ./...` from
the repository root as well.
