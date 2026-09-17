/* The project and case records this window can read, held between visits.
 *
 * ⛔ THERE IS NO TIMER IN THIS FILE AND THERE MUST NOT BE ONE. Section 11
 * requirement 8 wants real-time and Boris deferred it in the same breath:
 * "We can have the data displayed without live updates until the mechanism is
 * ready." The mechanism is section 5h's bus at M13. A poll added here would
 * become the trap section 11 names by name - every consumer written against
 * it would believe it was event-driven, and nothing would say otherwise the
 * day the bus lands. The 3s registry poll in App.svelte is older than this
 * file and is deliberately NOT extended to this data.
 *
 * WHY A STORE AND NOT A PROP. The dashboard and the project/case GUI show the
 * same read. Two components dialling separately would put two answers on
 * screen that could disagree, with nothing to tell a reader which was older.
 * One read per project, one timestamp per read, one explicit refresh.
 */

import * as RigService from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/rigservice.js";
import type {
  Brief,
  ProjectRef,
} from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";

/* The only name this window can ask about when the roster comes back empty.
 *
 * See cmd/rigwindow/brief.go's note on Projects(): no verb on this wire
 * enumerates projects - record.query needs a project name, which is circular -
 * so the roster IS empty today. Exported so the view can say it is a fallback
 * rather than presenting it as something that was discovered.
 */
export const SELF = "rig";

export type Held = {
  brief: Brief | null;
  error: string;
  readAt: string;
};

const EMPTY: Held = { brief: null, error: "", readAt: "" };

export type RigStore = {
  /** Every project or case the store could name. Empty today; see SELF. */
  readonly projects: ProjectRef[];
  /** True once the roster has been asked for, however it answered. */
  readonly rosterAsked: boolean;
  /** Whatever went wrong asking for the roster, verbatim. */
  readonly rosterError: string;
  /** Which project or case the GUI is looking at. */
  readonly current: string;
  readonly loading: boolean;
  /** The held read for a name, or an empty one. Never throws. */
  held(name: string): Held;
  /** Discover the roster, then read `name`. Safe to call repeatedly. */
  open(name?: string): Promise<void>;
  /** Read again, ignoring what is held. What the refresh control calls. */
  refresh(name?: string): Promise<void>;
  /** Roster and one held brief, for the contrast gate. The gate runs in a
      browser with no Wails runtime and would otherwise measure a blank page. */
  seed(
    projects: ProjectRef[],
    name: string,
    brief: Brief,
    readAt: string,
  ): void;
};

export function createRigStore(): RigStore {
  let projects = $state<ProjectRef[]>([]);
  let rosterAsked = $state(false);
  let rosterError = $state("");
  let current = $state(SELF);
  let loading = $state(false);
  let cache = $state<Record<string, Held>>({});
  let seeded = false;

  async function roster() {
    if (rosterAsked) return;
    rosterAsked = true;
    try {
      projects = (await RigService.Projects()) ?? [];
    } catch (e) {
      projects = [];
      rosterError = String(e);
    }
  }

  async function read(name: string) {
    loading = true;
    try {
      const b = await RigService.Brief(name);
      cache = {
        ...cache,
        [name]: {
          brief: b,
          error: "",
          readAt: new Date().toLocaleTimeString([], { hour12: false }),
        },
      };
    } catch (e) {
      cache = {
        ...cache,
        [name]: { brief: null, error: String(e), readAt: "" },
      };
    }
    loading = false;
  }

  return {
    get projects() {
      return projects;
    },
    get rosterAsked() {
      return rosterAsked;
    },
    get rosterError() {
      return rosterError;
    },
    get current() {
      return current;
    },
    get loading() {
      return loading;
    },
    held(name: string) {
      return cache[name] ?? EMPTY;
    },
    async open(name: string = current) {
      if (seeded) {
        current = name;
        return;
      }
      await roster();
      current = name;
      // Held reads are kept. Switching back to a tab must not re-dial: with
      // live updates deferred, a read is a snapshot a person asked for, and
      // silently refreshing one behind their back is the poll this file
      // refuses by another route.
      if (!cache[name]) await read(name);
    },
    async refresh(name: string = current) {
      if (seeded) return;
      await read(name);
    },
    seed(p: ProjectRef[], name: string, brief: Brief, readAt: string) {
      seeded = true;
      projects = p;
      rosterAsked = true;
      current = name;
      cache = { [name]: { brief, error: "", readAt } };
    },
  };
}
