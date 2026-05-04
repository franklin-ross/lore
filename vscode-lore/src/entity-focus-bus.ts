import * as vscode from "vscode";

export interface EntityFocus {
  entity: string;
  source: string | undefined;
  origin: "wiki" | "graph" | "external";
}

/**
 * Cross-panel coordination for the active entity. The wiki panel fires when
 * the user navigates to an entity page; the graph panel fires when the user
 * clicks a node. Each subscriber filters by `origin` so it doesn't react to
 * its own emissions and start a feedback loop.
 */
export class EntityFocusBus {
  private readonly emitter = new vscode.EventEmitter<EntityFocus>();
  readonly onDidFocus = this.emitter.event;

  fire(focus: EntityFocus): void {
    this.emitter.fire(focus);
  }

  dispose(): void {
    this.emitter.dispose();
  }
}
