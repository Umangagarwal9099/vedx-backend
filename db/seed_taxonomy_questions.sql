-- ============================================================================
-- Seed questions across the question-bank taxonomy (Subject -> Topic -> Subtopic)
-- introduced in schema_updates_question_bank_taxonomy_v1.sql /
-- schema_updates_coding_question_taxonomy_v1.sql.
--
-- Requires those two migrations to have run first. created_by resolves to
-- your first super_admin at insert time. Safe to run multiple times — every
-- short_id is freshly randomized, so re-running just adds more rows rather
-- than erroring, but you likely only want to run this once.
-- ============================================================================

-- ── PYTHON ──────────────────────────────────────────────────────────────────

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'Which keyword is used to define a function in Python?',
    '[{"id":"a","text":"func"},{"id":"b","text":"def"},{"id":"c","text":"function"},{"id":"d","text":"lambda"}]'::jsonb,
    ARRAY['b'], 5, 1, 'Python', 'Functions', 'Function Definition', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'What is the output of print(type([]))?',
    '[{"id":"a","text":"<class ''list''>"},{"id":"b","text":"<class ''tuple''>"},{"id":"c","text":"<class ''dict''>"},{"id":"d","text":"<class ''set''>"}]'::jsonb,
    ARRAY['a'], 5, 1, 'Python', 'Collections', 'List', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'multi_select',
    'Which of these are mutable data types in Python? (select all that apply)',
    '[{"id":"a","text":"list"},{"id":"b","text":"tuple"},{"id":"c","text":"dict"},{"id":"d","text":"set"}]'::jsonb,
    ARRAY['a','c','d'], 10, 2, 'Python', 'Collections', 'Dictionary', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'true_false',
    'In Python, a for loop can iterate directly over a dictionary''s keys without calling .keys().',
    '[{"id":"true","text":"True"},{"id":"false","text":"False"}]'::jsonb,
    ARRAY['true'], 3, 0, 'Python', 'Control Flow', 'For Loop', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'fill_blank',
    'The ______ keyword is used to handle exceptions in Python before the exception type is specified.',
    '[]'::jsonb, ARRAY[]::text[], 'except', 5, 1, 'Python', 'Advanced Python', 'Exception Handling', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'short_answer',
    'Explain the difference between a list and a tuple in Python.',
    '[]'::jsonb, ARRAY[]::text[], 'Lists are mutable and defined with [], tuples are immutable and defined with ().', 10, 0, 'Python', 'Collections', 'Tuple', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'descriptive',
    'Describe how Python''s garbage collector uses reference counting and explain a scenario where it can fail (circular references).',
    '[]'::jsonb, ARRAY[]::text[], 'Reference counting frees an object once its count hits zero; circular references never reach zero without the cyclic garbage collector.', 15, 0, 'Python', 'Advanced Python', 'Multithreading', 'hard', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'Which of these correctly demonstrates inheritance in Python?',
    '[{"id":"a","text":"class Dog(Animal):"},{"id":"b","text":"class Dog extends Animal:"},{"id":"c","text":"class Dog: inherits Animal"},{"id":"d","text":"class Dog -> Animal:"}]'::jsonb,
    ARRAY['a'], 5, 1, 'Python', 'OOP', 'Inheritance', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- ── JAVA ────────────────────────────────────────────────────────────────────

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'Which access modifier makes a member visible only within its own class?',
    '[{"id":"a","text":"public"},{"id":"b","text":"protected"},{"id":"c","text":"private"},{"id":"d","text":"default"}]'::jsonb,
    ARRAY['c'], 5, 1, 'Java', 'OOP', 'Encapsulation', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'Which collection class does NOT allow duplicate elements?',
    '[{"id":"a","text":"ArrayList"},{"id":"b","text":"LinkedList"},{"id":"c","text":"HashSet"},{"id":"d","text":"HashMap values"}]'::jsonb,
    ARRAY['c'], 5, 1, 'Java', 'Collections', 'HashSet', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'multi_select',
    'Which of these are valid ways to achieve polymorphism in Java? (select all that apply)',
    '[{"id":"a","text":"Method Overloading"},{"id":"b","text":"Method Overriding"},{"id":"c","text":"Variable Shadowing"},{"id":"d","text":"Static Binding of interfaces"}]'::jsonb,
    ARRAY['a','b'], 10, 2, 'Java', 'OOP', 'Polymorphism', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'true_false',
    'In Java, an interface can have default methods with a body.',
    '[{"id":"true","text":"True"},{"id":"false","text":"False"}]'::jsonb,
    ARRAY['true'], 3, 0, 'Java', 'OOP', 'Interfaces', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'fill_blank',
    'The ______ keyword is used to handle exceptions that might be thrown by a block of code in Java.',
    '[]'::jsonb, ARRAY[]::text[], 'try', 5, 1, 'Java', 'Advanced Java', 'Exception Handling', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'short_answer',
    'What is the difference between method overloading and method overriding in Java?',
    '[]'::jsonb, ARRAY[]::text[], 'Overloading is same method name with different parameters in the same class; overriding is redefining a parent method in a subclass with the same signature.', 10, 0, 'Java', 'Methods', 'Overriding', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'descriptive',
    'Explain how Java''s garbage collector determines an object is eligible for collection, and describe the generational GC model.',
    '[]'::jsonb, ARRAY[]::text[], 'An object becomes eligible once it is unreachable from any GC root; the generational model splits the heap into Young/Old generations since most objects die young.', 15, 0, 'Java', 'Advanced Java', 'Multithreading', 'hard', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'Which Java feature allows a class to work with any data type without casting?',
    '[{"id":"a","text":"Generics"},{"id":"b","text":"Reflection"},{"id":"c","text":"Autoboxing"},{"id":"d","text":"Annotations"}]'::jsonb,
    ARRAY['a'], 5, 1, 'Java', 'Advanced Java', 'Generics', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- ── C ───────────────────────────────────────────────────────────────────────

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'Which operator is used to get the memory address of a variable in C?',
    '[{"id":"a","text":"*"},{"id":"b","text":"&"},{"id":"c","text":"#"},{"id":"d","text":"@"}]'::jsonb,
    ARRAY['b'], 5, 1, 'C', 'Pointers & Memory', 'Pointers', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'Which function is used to dynamically allocate memory in C?',
    '[{"id":"a","text":"new()"},{"id":"b","text":"alloc()"},{"id":"c","text":"malloc()"},{"id":"d","text":"create()"}]'::jsonb,
    ARRAY['c'], 5, 1, 'C', 'Pointers & Memory', 'Dynamic Memory Allocation', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'true_false',
    'In C, array indices start at 1 by default.',
    '[{"id":"true","text":"True"},{"id":"false","text":"False"}]'::jsonb,
    ARRAY['false'], 3, 0, 'C', 'Pointers & Memory', 'Pointers', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'fill_blank',
    'The ______ function is used to free dynamically allocated memory in C.',
    '[]'::jsonb, ARRAY[]::text[], 'free', 5, 1, 'C', 'Pointers & Memory', 'Dynamic Memory Allocation', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'descriptive',
    'Explain the difference between a stack and heap memory allocation in C, and when you would use each.',
    '[]'::jsonb, ARRAY[]::text[], 'Stack is automatically managed, fast, and limited in size (local variables); heap is manually managed via malloc/free, larger, and used for dynamic data.', 15, 0, 'C', 'Pointers & Memory', 'Dynamic Memory Allocation', 'hard', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- ── REACT ───────────────────────────────────────────────────────────────────

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'Which hook lets you add state to a functional component?',
    '[{"id":"a","text":"useEffect"},{"id":"b","text":"useState"},{"id":"c","text":"useRef"},{"id":"d","text":"useMemo"}]'::jsonb,
    ARRAY['b'], 5, 1, 'React', 'Hooks', 'useState', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'Which hook runs side effects after render, such as data fetching?',
    '[{"id":"a","text":"useEffect"},{"id":"b","text":"useState"},{"id":"c","text":"useCallback"},{"id":"d","text":"useReducer"}]'::jsonb,
    ARRAY['a'], 5, 1, 'React', 'Hooks', 'useEffect', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'multi_select',
    'Which of these hooks are used for performance optimization? (select all that apply)',
    '[{"id":"a","text":"useMemo"},{"id":"b","text":"useCallback"},{"id":"c","text":"useState"},{"id":"d","text":"useContext"}]'::jsonb,
    ARRAY['a','b'], 10, 2, 'React', 'Hooks', 'useMemo', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'true_false',
    'Props in React are mutable and can be changed directly by the child component.',
    '[{"id":"true","text":"True"},{"id":"false","text":"False"}]'::jsonb,
    ARRAY['false'], 3, 0, 'React', 'Props', 'Passing Props', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'fill_blank',
    'The ______ hook lets a component subscribe to data from a Context Provider without prop drilling.',
    '[]'::jsonb, ARRAY[]::text[], 'useContext', 5, 1, 'React', 'State Management', 'Context API', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'short_answer',
    'What is the dependency array in useEffect used for?',
    '[]'::jsonb, ARRAY[]::text[], 'It tells React when to re-run the effect — the effect only re-runs when one of the listed values changes.', 10, 0, 'React', 'Hooks', 'useEffect', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'descriptive',
    'Explain how React''s reconciliation algorithm decides whether to re-use or replace DOM nodes when a list is re-rendered, and why the "key" prop matters.',
    '[]'::jsonb, ARRAY[]::text[], 'React diffs elements by type+key; a stable, unique key lets it match old/new elements correctly instead of re-mounting/re-ordering incorrectly.', 15, 0, 'React', 'Components', 'JSX', 'hard', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- ── FULL STACK ──────────────────────────────────────────────────────────────

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'Which HTTP method is idempotent and used to fully replace a resource?',
    '[{"id":"a","text":"POST"},{"id":"b","text":"PUT"},{"id":"c","text":"PATCH"},{"id":"d","text":"CONNECT"}]'::jsonb,
    ARRAY['b'], 5, 1, 'Full Stack', 'REST APIs', 'Endpoints', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'mcq',
    'In Express.js, what does app.use() register?',
    '[{"id":"a","text":"A database connection"},{"id":"b","text":"Middleware"},{"id":"c","text":"A route only"},{"id":"d","text":"A static file only"}]'::jsonb,
    ARRAY['b'], 5, 1, 'Full Stack', 'Express', 'Middleware', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'multi_select',
    'Which of these are valid CSS layout systems? (select all that apply)',
    '[{"id":"a","text":"Flexbox"},{"id":"b","text":"Grid"},{"id":"c","text":"Float"},{"id":"d","text":"Padding"}]'::jsonb,
    ARRAY['a','b','c'], 10, 2, 'Full Stack', 'CSS', 'Flexbox', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'true_false',
    'MongoDB is a relational database that requires a fixed schema.',
    '[{"id":"true","text":"True"},{"id":"false","text":"False"}]'::jsonb,
    ARRAY['false'], 3, 0, 'Full Stack', 'MongoDB', 'Documents', 'easy', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'fill_blank',
    'A SQL ______ JOIN returns only rows that have matching values in both tables.',
    '[]'::jsonb, ARRAY[]::text[], 'INNER', 5, 1, 'Full Stack', 'SQL', 'Joins', 'medium', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO assessment_questions (short_id, question_type, question_text, options, correct_option_ids, correct_text, marks, negative_marks, subject, topic, subtopic, difficulty, visibility, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8), 'descriptive',
    'Describe the request/response lifecycle when a browser submits a form to a REST API backed by Node.js/Express and a SQL database, including where authentication would typically be checked.',
    '[]'::jsonb, ARRAY[]::text[], 'Browser sends HTTP request -> Express middleware chain (auth check, body parsing) -> route handler -> DB query -> response serialized back to the client.', 15, 0, 'Full Stack', 'REST APIs', 'Authentication', 'hard', 'global',
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

-- ============================================================================
-- Coding questions — same Subject/Subtopic taxonomy, real problems + test cases
-- ============================================================================

INSERT INTO coding_questions (short_id, title, description, difficulty, subject, subtopic, topics, languages, constraints, examples, starter_code, test_cases, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'Two Sum',
    'Given an array of integers nums and an integer target, return the indices of the two numbers that add up to target. Each input has exactly one solution, and you may not use the same element twice.',
    'Easy', 'Python', 'Dictionary',
    ARRAY['Arrays','Hash Map'], ARRAY['python','java'],
    ARRAY['2 <= nums.length <= 10^4', '-10^9 <= nums[i] <= 10^9'],
    '[{"input":"nums = [2,7,11,15], target = 9","output":"[0,1]","explanation":"nums[0] + nums[1] = 2 + 7 = 9"}]'::jsonb,
    '{"python":"nums = list(map(int, input().split()))\ntarget = int(input())\n# Write your solution below\n","java":"import java.util.Scanner;\npublic class Main {\n    public static void main(String[] args) {\n        // Write your solution below\n    }\n}\n"}'::jsonb,
    '[{"input":"2 7 11 15\n9","expected_output":"0 1","is_hidden":false},{"input":"3 2 4\n6","expected_output":"1 2","is_hidden":true}]'::jsonb,
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO coding_questions (short_id, title, description, difficulty, subject, subtopic, topics, languages, constraints, examples, starter_code, test_cases, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'Reverse a Linked List',
    'Given the head of a singly linked list (as space-separated integers), reverse the list and print the reversed sequence.',
    'Medium', 'Python', 'Recursion',
    ARRAY['Linked List','Recursion'], ARRAY['python','java'],
    ARRAY['0 <= list length <= 5000'],
    '[{"input":"1 2 3 4 5","output":"5 4 3 2 1"}]'::jsonb,
    '{"python":"vals = list(map(int, input().split()))\n# Write your solution below\n","java":"import java.util.Scanner;\npublic class Main {\n    public static void main(String[] args) {\n        // Write your solution below\n    }\n}\n"}'::jsonb,
    '[{"input":"1 2 3 4 5","expected_output":"5 4 3 2 1","is_hidden":false}]'::jsonb,
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO coding_questions (short_id, title, description, difficulty, subject, subtopic, topics, languages, constraints, examples, starter_code, test_cases, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'Check Balanced Parentheses',
    'Given a string containing just the characters ( ) { } [ ], determine if the input string is valid (every opening bracket has a matching closing bracket in the correct order).',
    'Medium', 'Java', 'HashSet',
    ARRAY['Stack','Strings'], ARRAY['python','java'],
    ARRAY['1 <= s.length <= 10^4'],
    '[{"input":"()[]{}","output":"true"},{"input":"(]","output":"false"}]'::jsonb,
    '{"python":"s = input()\n# Write your solution below\n","java":"import java.util.Scanner;\npublic class Main {\n    public static void main(String[] args) {\n        // Write your solution below\n    }\n}\n"}'::jsonb,
    '[{"input":"()[]{}","expected_output":"true","is_hidden":false},{"input":"(]","expected_output":"false","is_hidden":true}]'::jsonb,
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO coding_questions (short_id, title, description, difficulty, subject, subtopic, topics, languages, constraints, examples, starter_code, test_cases, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'Find the Largest Prime Factor',
    'Given a positive integer n, print its largest prime factor.',
    'Medium', 'C', 'Pointers',
    ARRAY['Math','Number Theory'], ARRAY['python','java'],
    ARRAY['2 <= n <= 10^12'],
    '[{"input":"13195","output":"29"}]'::jsonb,
    '{"python":"n = int(input())\n# Write your solution below\n","java":"import java.util.Scanner;\npublic class Main {\n    public static void main(String[] args) {\n        // Write your solution below\n    }\n}\n"}'::jsonb,
    '[{"input":"13195","expected_output":"29","is_hidden":false},{"input":"600851475143","expected_output":"6857","is_hidden":true}]'::jsonb,
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);

INSERT INTO coding_questions (short_id, title, description, difficulty, subject, subtopic, topics, languages, constraints, examples, starter_code, test_cases, created_by)
SELECT substr(md5(random()::text || clock_timestamp()::text), 1, 8),
    'Debounce a Function Call (Simulated)',
    'Given a sequence of timestamps (integers, ms) at which a function is "called", and a debounce delay d, print the timestamps at which the function would actually execute if debounced by d ms (only the last call in any burst within d ms of each other fires).',
    'Hard', 'React', 'useEffect',
    ARRAY['Simulation','Frontend Concepts'], ARRAY['python','java'],
    ARRAY['1 <= calls <= 10^4'],
    '[{"input":"0 50 100 500\n100","output":"100 500"}]'::jsonb,
    '{"python":"times = list(map(int, input().split()))\nd = int(input())\n# Write your solution below\n","java":"import java.util.Scanner;\npublic class Main {\n    public static void main(String[] args) {\n        // Write your solution below\n    }\n}\n"}'::jsonb,
    '[{"input":"0 50 100 500\n100","expected_output":"100 500","is_hidden":false}]'::jsonb,
    (SELECT id FROM users WHERE role = 'super_admin' ORDER BY created_at LIMIT 1);
