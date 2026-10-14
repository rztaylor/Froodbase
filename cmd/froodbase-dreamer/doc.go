// Command froodbase-dreamer is the background module that works on stored knowledge.
//
// It owns process startup for the job workers: index maintenance, embeddings, reconsideration of derived records, review sweeps, gap scans, consolidation, reflection and knowledge views. It never serves callers and never accepts generated output as anything but a proposal.
// Keep main small: parse flags, wire dependencies from internal packages, and
// map errors to exit codes.
package main
