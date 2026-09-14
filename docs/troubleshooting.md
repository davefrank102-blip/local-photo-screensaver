# Troubleshooting

| Symptom | Check |
| --- | --- |
| Roku stuck on Loading / error | PC IP, firewall 8787, photoserver running, same Wi‑Fi |
| Sideload “No Development Application” | Zip must use `/` paths; `manifest` at zip root |
| Compile error on `bs_const` | Do not set `bs_const=true` in manifest |
| Gray freeze (old builds) | Never call `roUrlTransfer` on the render thread; use Task / main thread |
| Long pause every 50 photos | Use build ≥10 (prefetch next page early) |
| Sideways photos | photoserver with EXIF auto-orient (current server) |
| Junk scan finds nothing | Heuristics only; camera JPEGs often won’t match — expected |
