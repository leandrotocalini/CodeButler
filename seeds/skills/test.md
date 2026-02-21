# test

Write tests for a specific part of the codebase.

## Trigger
write tests for {target}, test {target}, add tests for {target}

## Agent
coder

## Prompt
Write tests for {{target}}.

1. Find and read the source code for {{target}} — understand the public API, edge cases, and error paths
2. Check if tests already exist. If so, read them to understand the existing style and coverage
3. Follow the project's existing test patterns (test framework, naming, structure). If no tests exist yet, use the language's standard test library
4. Write tests covering:
   - Happy path (normal usage)
   - Edge cases (empty input, boundaries, nil/zero values)
   - Error cases (invalid input, expected failures)
5. Run the tests. Fix any failures
6. Create a PR with the new tests
