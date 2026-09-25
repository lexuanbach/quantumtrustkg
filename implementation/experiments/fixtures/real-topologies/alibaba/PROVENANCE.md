# Alibaba microservice call-graph topology (real data)

- **Source:** Alibaba Cluster Trace Program, `cluster-trace-microservices-v2022`
  (https://github.com/alibaba/clusterdata). Public production trace.
- **Subset used:** a single 3-minute `CallGraph` bucket (`CallGraph_0.tar.gz`,
  223 MB, 13,332,356 call rows) — one subset only; the full 13-day trace is ~2 TB
  and is intentionally NOT used.
- **Extraction:** real service-dependency edges are the unique caller->callee pairs
  (`um -> dm`, self-loops removed): **16,247 services, 48,746 edges**, heavy-tailed
  degree (median 3, max 2971) — a genuine production microservice topology.
- **Subgraphs:** connected BFS subgraphs from the highest-degree hub at the paper's
  scales: `alibaba_{50,100,200,500,1000,2000}.edges`.
- Crypto posture requirements / supported profiles are not present in the trace;
  they are assigned per edge by the calibrated model (as for the synthetic meshes).
  Only the *topology* is real.
