# Benchmark le plus rÃ©cent

GÃ©nÃ©rÃ© le : 2026-09-24T14:01:22.0607703Z (UTC)

Profils : `cpu-20260924-140122062.prof` (CPU), `memory-20260924-140122062.prof` (mÃ©moire), `gc-20260924-140122062.log` (GC)

| ParamÃ¨tre | Valeur |
| --- | ---: |
| Runs Go | 1 |
| Warmup Go | 0 |
| Hyperfine | oui (hyperfine-20260924-140122062.json) |
| Profil | RÃ©sultat |
| CPU | `cpu-20260924-140122062.prof` |
| MÃ©moire | `memory-20260924-140122062.prof` |
| GC dÃ©taillÃ© | `gc-20260924-140122062.log` |

## RÃ©sultat du tick sur la carte 600 Ã— 600

| Benchmark | Moyenne | P95 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| BenchmarkTick-16 | 12.395 ms | 12.395 ms | 12395032 | 3 | 0 |

## Notes

- Mesure du calcul du tick sur une carte 600 Ã— 600.
- DensitÃ© de la carte : 0.25 ; seed : 42.
- La latence API est mesurÃ©e Ã  part dans le frontend, sans rendu canvas.
- La trace GC dÃ©taillÃ©e est enregistrÃ©e dans le fichier `gc-*.log` et rÃ©sumÃ©e dans le PDF.
- Les mÃ©triques I/O, rÃ©seau et base de donnÃ©es ne sont pas instrumentÃ©es dans ce benchmark.
