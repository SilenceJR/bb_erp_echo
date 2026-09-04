/** Returns a target only when focus must wrap (or has already escaped the loop). */
export function resolveFocusLoopTarget(
  itemCount: number,
  currentIndex: number,
  backwards: boolean,
): number | null {
  if (itemCount <= 0) return null
  if (currentIndex < 0 || currentIndex >= itemCount) return backwards ? itemCount - 1 : 0
  if (backwards && currentIndex === 0) return itemCount - 1
  if (!backwards && currentIndex === itemCount - 1) return 0
  return null
}
