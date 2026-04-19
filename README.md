# market-research

An agentic market-research system. Given a topic, it surfaces live pain points from public platforms, forward-projects where the pain is heading, generates differentiated solutions that are ahead of the current market state, and generates tooling scaffolds for those solutions.

## Product decomposition

This product is not a single project. It is five subsystems, each built and specced independently:

| # | Subsystem | What it does | Status |
|---|---|---|---|
| 1 | **Data collection** | Pulls raw signal from Reddit and Stack Overflow into local SQLite. Agent-discovered sources per topic, daily batch, weekly source rediscovery. | Spec approved · plan pending |
| 2 | **Pain-point extraction** | LLM-driven categorization of problems from raw signal. | Not specced |
| 3 | **Trend forward-projection** | Models where a pain point is heading and how fast it is growing. | Not specced |
| 4 | **Solution generation** | Generates differentiated solutions ahead of the current market state. | Not specced |
| 5 | **Tool generation** | Generates implementation scaffolds for generated solutions. | Not specced |

Each subsystem gets its own design spec in `docs/superpowers/specs/`, its own implementation plan, its own implementation pass. Subsystems 2-5 read from the data layer via the SQLite schema defined in subsystem #1 - that schema is the durable contract between them.

## v1 constraints

- **Platforms:** Reddit + Stack Overflow only. Google Trends and X deferred.
- **Language:** Go.
- **Storage:** SQLite (schema-as-contract handoff to downstream subsystems).
- **Deployment:** Tiny always-on VM, orchestrated by systemd.
- **Architecture:** CLI + systemd timers (no in-process scheduler).

## Specs

- [2026-04-18 - Data Collection Layer](docs/superpowers/specs/2026-04-18-data-collection-design.md)

## Status

Pre-implementation. Spec for subsystem #1 is approved; implementation plan is next.
