package domain

// Corroboration counts the distinct evidence roots among the given evidence
// (PR-12, TR-25): a report, an excerpt from it and a summary of both are one
// line of evidence, not three. Roots are resolved through the records present
// in the slice, so a restatement of a restatement still counts once; a root
// that is not itself in the slice counts as one root.
func Corroboration(evidence []Evidence) int {
	return len(DistinctRoots(evidence))
}

// resolveRoot follows RootEvidenceID through the known records until it
// reaches an original or leaves the set. A cycle (malformed data) resolves to
// the smallest id on the cycle itself, so every record that leads into the
// same cycle counts once whatever its starting point.
func resolveRoot(byID map[ID]Evidence, e Evidence) ID {
	seen := map[ID]bool{}
	cur := e
	for {
		seen[cur.ID] = true
		root := cur.Root()
		if root == cur.ID {
			return root
		}
		next, ok := byID[root]
		if !ok {
			return root
		}
		if seen[next.ID] {
			return smallestOnCycle(byID, next)
		}
		cur = next
	}
}

// smallestOnCycle walks the cycle that start lies on and returns its smallest id.
func smallestOnCycle(byID map[ID]Evidence, start Evidence) ID {
	smallest := start.ID
	for cur := byID[start.Root()]; cur.ID != start.ID; cur = byID[cur.Root()] {
		if cur.ID < smallest {
			smallest = cur.ID
		}
	}
	return smallest
}
