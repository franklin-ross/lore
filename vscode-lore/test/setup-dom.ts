// Installs a happy-dom Window onto globalThis so the wiki webview modules
// can use document/HTMLElement APIs from a Node test process. Imported for
// side-effects at the top of every webview-rendering test file.
import { Window } from "happy-dom";

const window = new Window();
const g = globalThis as unknown as Record<string, unknown>;
g.window = window;
g.document = window.document;
g.HTMLElement = window.HTMLElement;
g.HTMLInputElement = window.HTMLInputElement;
g.Node = window.Node;
g.Text = window.Text;
g.URL = window.URL;
g.MouseEvent = window.MouseEvent;
g.KeyboardEvent = window.KeyboardEvent;
g.requestAnimationFrame = (cb: FrameRequestCallback): number => {
  setTimeout(() => cb(performance.now()), 0);
  return 0;
};

export function resetDom(): void {
  window.document.body.innerHTML = "";
  window.document.head.innerHTML = "";
}
