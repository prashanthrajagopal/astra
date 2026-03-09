# SimpleAgent Example

Run Astra services first:

```bash
./scripts/deploy.sh
```

Run example:

```bash
go run ./examples/simple-agent
```

This demonstrates:

- Plan -> returns one task
- Execute -> calls tool-runtime
- Reflect -> writes memory entry
