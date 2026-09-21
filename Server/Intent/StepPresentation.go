package Intent

import "CitadelDesktop/Server/Localization"

// WithNameDescriptor adds presentation metadata without changing command semantics.
// Binding clones the message so reusable source descriptors remain immutable.
func (step Step) WithNameDescriptor(message *Localization.Message) Step {
	step.NameDescriptor = Localization.Bind(message, step.Name)
	return step
}
