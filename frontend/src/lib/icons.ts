/* A program's icon, by name (plan/11, Boris 2026-09-27: "storeworker is not
 * just `st`, it should have a nice icon and when hovered should say its name.
 * The icon should be meaningful.").
 *
 * `Identity.icon` on the wire is a NAME from this set, never markup, so a
 * program cannot put SVG into the window. The set is Lucide's (ISC), imported
 * one icon at a time so the bundle carries only these. A name the set does not
 * know keeps the program's two letters: nothing here guesses an icon from an
 * id, because that would fake a declaration the program never made.
 *
 * To offer a program a new icon, add its Lucide name here.
 */
import {
  Archive,
  BookOpen,
  Boxes,
  Briefcase,
  Calculator,
  Calendar,
  ClipboardList,
  Database,
  FileText,
  FlaskConical,
  Folder,
  Gauge,
  GitPullRequest,
  Globe,
  Hand,
  HeartPulse,
  Inbox,
  LayoutDashboard,
  Library,
  Lightbulb,
  ListTodo,
  Mail,
  Package,
  Scale,
  Server,
  Terminal,
  Wallet,
  Warehouse,
  Workflow,
  Wrench,
  type IconNode,
} from "lucide";

export type { IconNode };

const SET: Record<string, IconNode> = {
  archive: Archive,
  "book-open": BookOpen,
  boxes: Boxes,
  briefcase: Briefcase,
  calculator: Calculator,
  calendar: Calendar,
  "clipboard-list": ClipboardList,
  database: Database,
  "file-text": FileText,
  "flask-conical": FlaskConical,
  folder: Folder,
  gauge: Gauge,
  "git-pull-request": GitPullRequest,
  globe: Globe,
  hand: Hand,
  "heart-pulse": HeartPulse,
  inbox: Inbox,
  "layout-dashboard": LayoutDashboard,
  library: Library,
  lightbulb: Lightbulb,
  "list-todo": ListTodo,
  mail: Mail,
  package: Package,
  scale: Scale,
  server: Server,
  terminal: Terminal,
  wallet: Wallet,
  warehouse: Warehouse,
  workflow: Workflow,
  wrench: Wrench,
};

/** The drawing for a declared icon name, or null when the set has none. */
export function programIcon(name: string | undefined): IconNode | null {
  if (!name || !Object.hasOwn(SET, name)) return null;
  return SET[name];
}

/** The two letters a program without a known icon keeps: its icon when that
 *  is two letters already, the older form of the field, else its id's. */
export function programGlyph(icon: string | undefined, id: string): string {
  return icon && icon.length <= 2 ? icon : id.slice(0, 2);
}
