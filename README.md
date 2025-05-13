# go-scripts

Misc go scripts. 

## Validator Registry (simple stake contract) migration history

`ValidatorRegistry.go` is the go binding of the initial contract that was deployed on mev-commit chain.
`ValidatorRegistryV1.go` is the binding of holesky contract that served as the source of truth of validator opt-in until August 15th 2024. `ValidatorRegistryV1_aug15.go` is the binding of the holesky contract that's up to date with mev-commit main branch commit `ffd2f19bd9abd0cdbe6d9a094d88a4aa21432ba3`, that will serve as the source of truth for validator opt-in from August 15th 2024 onwards. See https://github.com/primev/mev-commit/commit/ffd2f19bd9abd0cdbe6d9a094d88a4aa21432ba3.

`cmd/holesky-migrate` is the tool for the most recent migration that started on August 15th 2024.
