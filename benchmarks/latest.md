# Benchmark le plus récent

| Mesure | Valeur |
| --- | ---: |
| Runs | 10 |
| Warmup | 2 |
| Latence moyenne | 0.548 ms |
| Médiane | 0.563 ms |
| P95 | 0.605 ms |
| P99 | 0.605 ms |
| Maximum | 0.629 ms |
| ns/op | 548374 |
| B/op | 397529 |
| allocs/op | 16 |
| Débit | 1824 op/s |

## Notes

- Mesure du moteur Go sur 10 exécutions indépendantes.
- La latence API est mesurée à part dans le frontend, sans rendu canvas.
- Les métriques CPU bas niveau, GC détaillé, I/O, réseau et base de données ne sont pas instrumentées dans ce benchmark.
