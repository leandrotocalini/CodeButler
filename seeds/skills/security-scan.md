# security-scan

Run a security audit on the codebase or a specific area.

## Trigger
security scan, security audit, check for vulnerabilities, scan {target} for security

## Agent
reviewer

## Prompt
Run a security audit on {{target | default: "the entire codebase"}}.

1. Identify the attack surface: user inputs, API endpoints, authentication, authorization, data storage, external integrations
2. Check for OWASP Top 10 issues:
   - Injection (SQL, command, template)
   - Broken authentication / session management
   - Sensitive data exposure (secrets in code, logs, error messages)
   - Security misconfiguration (default credentials, debug mode, open CORS)
   - XSS (if web UI exists)
   - Insecure deserialization
   - Components with known vulnerabilities (check dependency versions)
3. Check for project-specific risks:
   - Secrets or API keys in committed files
   - Overly permissive file/directory permissions
   - Missing input validation at system boundaries
   - Unencrypted sensitive data in transit or at rest
4. Post a structured report in the thread:
   - **Critical** — must fix before deploy (with file:line references)
   - **Warning** — should fix soon
   - **Info** — best practice recommendations
   - **Clean** — areas that look good (briefly acknowledge)
5. For each finding, include: what the risk is, where it is, and a concrete fix suggestion
