---
name: implementation-planning
description: Plan implementation phases for new features following a structured phase, file, test, and dependency template
user-invocable: true
---

# Implementation Planning

## Task

Create a structured implementation plan for a new feature or change.

## Instructions

1. **Identify scope**: Determine which parts of the project this change affects
2. **Check dependencies**: Verify required types, modules, and components exist
3. **Plan file changes**: List every file that needs creation or modification
4. **Define test strategy**: Specify what unit tests, stories, and E2E tests are needed
5. **Estimate impact**: Note which existing components are affected

## Plan Template

```
### Feature: [name]
### Phase: [phase number and name]

#### Files to Create
- [ ] path/to/file - description

#### Files to Modify
- [ ] path/to/file - what changes

#### Tests Required
- [ ] Unit: description
- [ ] Story: description
- [ ] E2E: description (if applicable)

#### Dependencies
- Requires: [list existing modules needed]
- Blocks: [list what this unblocks]
```
