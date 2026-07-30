import type { Metadata } from "next";
import { ConceptShowcase } from "./ConceptShowcase";

export const metadata: Metadata = {
  title: "CitadelOps — Redesign Concept",
  description:
    "An evidence-led product, brand, and interaction concept for a calm game automation command center.",
};

export default function Home() {
  return <ConceptShowcase />;
}
