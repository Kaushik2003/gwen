import { createContext, useContext } from "react";

/** Which screen is shown; param picks something on it, such as a Settings section. */
export interface Nav {
  screen: string;
  param: string | null;
  go: (screen: string, param?: string | null) => void;
}

export const NavContext = createContext<Nav>({ screen: "today", param: null, go: () => {} });

export function useNav(): Nav {
  return useContext(NavContext);
}
