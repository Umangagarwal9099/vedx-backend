package models

import "time"

// QuestionBankTaxonomy is the reference Subject -> Topic -> Subtopic tree that
// drives the question bank's browse UI and the cascading selects in the
// question form. It's a fixed, code-defined tree rather than DB-editable rows
// — mentors can still free-type a subject/topic/subtopic that isn't in the
// tree (stored as-is), the tree is just the curated default picker.
type QuestionBankTaxonomy struct {
	Subject string              `json:"subject"`
	Topics  []QuestionBankTopic `json:"topics"`
	// ShortID is only populated when this tree came from the DB-backed subject
	// list (see QuestionBankController.GetTaxonomy) — needed so the admin UI
	// can target a subject for deletion.
	ShortID string `json:"short_id,omitempty"`
}

type QuestionBankTopic struct {
	Topic     string   `json:"topic"`
	Subtopics []string `json:"subtopics"`
}

// GetQuestionBankTaxonomy returns the full reference tree.
func GetQuestionBankTaxonomy() []QuestionBankTaxonomy {
	return questionBankTaxonomy
}

// QuestionBankSubject is one DB-editable subject row (the "Add Subject" flow) —
// what actually decides which tiles appear on the browse page today.
type QuestionBankSubject struct {
	ID           string    `json:"id"`
	ShortID      string    `json:"short_id"`
	Name         string    `json:"name"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
}

// TopicsForSubject returns the curated Topic/Subtopic tree for one of the
// original 5 subjects that shipped with a hand-built tree, or nil for any
// subject added later via the admin UI — those start with no preset topics;
// GetStats (real question data) fills the browse page's topic list instead.
func TopicsForSubject(name string) []QuestionBankTopic {
	for _, t := range questionBankTaxonomy {
		if t.Subject == name {
			return t.Topics
		}
	}
	return nil
}

var questionBankTaxonomy = []QuestionBankTaxonomy{
	{
		Subject: "Python",
		Topics: []QuestionBankTopic{
			{Topic: "Python Basics", Subtopics: []string{"Syntax", "Variables", "Data Types", "Operators"}},
			{Topic: "Control Flow", Subtopics: []string{"If-Else", "For Loop", "While Loop"}},
			{Topic: "Functions", Subtopics: []string{"Function Definition", "Parameters", "Return Values", "Lambda Functions"}},
			{Topic: "Collections", Subtopics: []string{"List", "Tuple", "Set", "Dictionary"}},
			{Topic: "OOP", Subtopics: []string{"Class and Object", "Inheritance", "Polymorphism", "Encapsulation"}},
			{Topic: "Advanced Python", Subtopics: []string{"Exception Handling", "File Handling", "Decorators", "Generators", "Multithreading"}},
		},
	},
	{
		Subject: "Java",
		Topics: []QuestionBankTopic{
			{Topic: "Java Basics", Subtopics: []string{"Syntax", "Variables", "Data Types", "Operators"}},
			{Topic: "Control Flow", Subtopics: []string{"If-Else", "For Loop", "While Loop", "Switch"}},
			{Topic: "Methods", Subtopics: []string{"Method Definition", "Parameters", "Overloading", "Overriding"}},
			{Topic: "Collections", Subtopics: []string{"ArrayList", "HashMap", "HashSet", "LinkedList"}},
			{Topic: "OOP", Subtopics: []string{"Class and Object", "Inheritance", "Polymorphism", "Encapsulation", "Interfaces", "Abstract Classes"}},
			{Topic: "Advanced Java", Subtopics: []string{"Exception Handling", "Multithreading", "Generics", "Streams", "Collections Framework"}},
		},
	},
	{
		Subject: "C",
		Topics: []QuestionBankTopic{
			{Topic: "C Basics", Subtopics: []string{"Syntax", "Variables", "Data Types", "Operators"}},
			{Topic: "Control Flow", Subtopics: []string{"If-Else", "For Loop", "While Loop", "Switch"}},
			{Topic: "Functions", Subtopics: []string{"Function Definition", "Parameters", "Recursion"}},
			{Topic: "Pointers & Memory", Subtopics: []string{"Pointers", "Arrays", "Dynamic Memory Allocation", "Structures"}},
			{Topic: "Advanced C", Subtopics: []string{"File Handling", "Preprocessor Directives", "Bitwise Operations"}},
		},
	},
	{
		Subject: "React",
		Topics: []QuestionBankTopic{
			{Topic: "Components", Subtopics: []string{"Functional Components", "Class Components", "JSX"}},
			{Topic: "Props", Subtopics: []string{"Passing Props", "PropTypes", "Default Props"}},
			{Topic: "State", Subtopics: []string{"useState", "Class State", "Lifting State Up"}},
			{Topic: "Events", Subtopics: []string{"Event Handlers", "Synthetic Events"}},
			{Topic: "Forms", Subtopics: []string{"Controlled Components", "Uncontrolled Components", "Form Validation"}},
			{Topic: "Hooks", Subtopics: []string{"useState", "useEffect", "useContext", "useMemo", "useCallback", "Custom Hooks"}},
			{Topic: "Routing", Subtopics: []string{"React Router", "Nested Routes", "Route Params"}},
			{Topic: "State Management", Subtopics: []string{"Context API", "Redux", "Zustand"}},
		},
	},
	{
		Subject: "Full Stack",
		Topics: []QuestionBankTopic{
			{Topic: "HTML", Subtopics: []string{"Structure", "Forms", "Semantic HTML"}},
			{Topic: "CSS", Subtopics: []string{"Selectors", "Flexbox", "Grid", "Responsive Design"}},
			{Topic: "JavaScript", Subtopics: []string{"ES6+", "Async/Await", "DOM Manipulation"}},
			{Topic: "Node.js", Subtopics: []string{"Modules", "Event Loop", "File System"}},
			{Topic: "Express", Subtopics: []string{"Routing", "Middleware", "Error Handling"}},
			{Topic: "SQL", Subtopics: []string{"Queries", "Joins", "Indexes"}},
			{Topic: "MongoDB", Subtopics: []string{"Documents", "Aggregation", "Indexing"}},
			{Topic: "REST APIs", Subtopics: []string{"Endpoints", "Status Codes", "Authentication"}},
			{Topic: "Deployment", Subtopics: []string{"CI/CD", "Docker", "Cloud Hosting"}},
		},
	},
}

// QuestionBankStats is the aggregate summary for one subject — powers the
// "Python – 540 Questions, MCQ: 310..." card on the browse page.
type QuestionBankStats struct {
	Subject        string         `json:"subject"`
	Total          int            `json:"total"`
	ByQuestionType map[string]int `json:"by_question_type"`
	ByDifficulty   map[string]int `json:"by_difficulty"`
	Topics         []TopicCount   `json:"topics"`
}

type TopicCount struct {
	Topic string `json:"topic"`
	Count int    `json:"count"`
}
