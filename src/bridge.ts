export type Item = Record<string, string>;
type Conflict = { ID: string; Field: string; Remote: string; Local: string };
export type State = {
  URL: string;
  Last: string;
  Message: string;
  Pending: boolean;
  PendingCount: number;
  Conflicts: Conflict[] | null;
  Items: Record<string, Item> | null;
};
export type Change = { ID: string; Before: Item | null; After: Item | null };
interface Bridge {
  Version(): Promise<string>;
  Mode(): Promise<boolean>;
  Settings(): Promise<void>;
  Close(): Promise<void>;
  State(refresh: boolean): Promise<State>;
  Save(c: Change[]): Promise<State>;
  Resolve(f: Conflict[], local: boolean): Promise<State>;
  Connect(url: string): Promise<void>;
  Days(
    items: Record<string, Item>,
    date: string,
    n: number,
  ): Promise<Record<string, string[]>>;
}
export const api = (window as unknown as { go: { main: { App: Bridge } } }).go
  .main.App;
