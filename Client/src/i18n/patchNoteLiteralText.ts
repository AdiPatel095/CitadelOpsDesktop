/** Literal technical syntax in release prose; ICU source quotes these braces. */
export const patchNoteLiteralText:Readonly<Record<string,string>> = {
  "patchNotes.release.2.3.0.text.fa6920e25e0b": "Dashboard commands no longer live or die with the dashboard connection: intents are accepted immediately and executed by the runtime under its own lifetime, completion streams through operation events with a polling fallback, and closing the tab, sleeping, losing the network, or a hosted gateway timeout can no longer cancel a running operation; POST /api/v2/intents/{name} now answers 202 with the accepted receipt unless ?wait=true asks for the final one"
};
