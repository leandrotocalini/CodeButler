# brainstorm

Multi-model brainstorming: fan out a question to multiple LLM models with different perspectives, then synthesize the best ideas.

## Trigger
brainstorm, brainstorm about {topic}, think about {topic} from multiple angles, get ideas for {topic}, multi-model {topic}

## Agent
pm

## Prompt
Run the brainstorm workflow for: {{topic | default: "ask user"}}

1. If the topic is vague, interview the user to clarify scope, constraints, and what kind of ideas they want
2. If the topic involves existing code, explore the codebase first — the Thinkers need real context to produce useful ideas
3. If the topic involves external tech, @codebutler.researcher for background before designing the panel
4. Design the Thinker panel: decide how many (2–6), craft a unique system prompt for each (persona + focus area + domain context), assign a different model to each from the multiModel.models pool
5. Post the panel design in the thread so the user knows what's coming
6. Call MultiModelFanOut with the per-Thinker configs
7. Post each Thinker's response in the thread (attributed)
8. Synthesize: common themes, unique insights, conflicts, ranked recommendations
9. Present the synthesis with clear attribution per idea
10. Ask: want to go deeper on any idea? Want to act on something? (→ discovery, roadmap-add, implement)
