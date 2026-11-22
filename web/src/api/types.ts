// One change on the event stream: which item of which module changed, and in which area.
export type Change = { module: string; kind: string; id: number; area: number }
