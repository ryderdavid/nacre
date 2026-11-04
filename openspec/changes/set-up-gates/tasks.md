## 1. Draft and save nacre.yaml (NAC-6c5)
- [x] 1.1 `POST /v1/repos/{repo}/config/draft`: read-only drafting call with computer facts and examples; reports a draft that doesn't parse
- [x] 1.2 `POST /v1/repos/{repo}/config`: validate, write without overwriting, commit only `nacre.yaml`
- [x] 1.3 Set up gates sheet in the app, offered before Start work, Run epic and Approve and run
- [x] 1.4 Gates: `go test ./...`, app unit tests, `openspec validate --all --strict`
