package core

type ProgrammingLanguage int

const (
	Golang ProgrammingLanguage = iota
	Python
	JavaScript
	Java
	Rust
	TypeScript
	// Add other programming languages here
)

func (p ProgrammingLanguage) String() string {
	switch p {
	case Golang:
		return "Golang"
	case Python:
		return "Python"
	case JavaScript:
		return "JavaScript"
	case Java:
		return "Java"
	case Rust:
		return "Rust"
	case TypeScript:
		return "TypeScript"
	default:
		return "Unknown"
	}
}
