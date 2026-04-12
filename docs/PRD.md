# Product Requirements Document: Jarvis-Dev MVP 1

**Version**: 1.0.0  
**Date**: 2026-04-10  
**Status**: Draft  
**Author**: Andrés (CTO Conpas)

---

## Executive Summary

Jarvis-Dev is an AI-powered development ecosystem designed specifically for the Conpas development team (8 developers). It addresses three critical pain points:

1. **Lack of shared knowledge**: Decisions, bug fixes, and architectural choices are siloed in individual developers' minds
2. **Weak QA process**: Zoho SaaS platform prevents automated testing, leading to manual testing chaos
3. **Inconsistent coding practices**: Each developer codes "however they want" without structure or methodology

**MVP 1 Scope**: Shared memory system (Hive) + Spec-Driven Development workflow (SDD) + AI personality customization + skill-based code standards.

**Timeline**: 3.5-5 months  
**Team**: 8 PHP/Zoho developers + 1 CTO (part-time)  
**Infrastructure**: 1 VPS (2GB RAM, 20GB disk) + 8 Claude Team seats

---

## Table of Contents

1. [Problem Statement](#problem-statement)
2. [Vision & Goals](#vision--goals)
3. [User Personas](#user-personas)
4. [Components Architecture](#components-architecture)
   - [Agent Support](#agent-support)
5. [Component 1: Hive (Shared Memory)](#component-1-hive-shared-memory)
6. [Component 2: SDD Workflow](#component-2-sdd-workflow)
7. [Component 3: Persona System](#component-3-persona-system)
8. [Component 4: Skill System](#component-4-skill-system)
9. [Technical Stack](#technical-stack)
10. [Database Schema](#database-schema)
11. [API Specification](#api-specification)
12. [CLI Commands](#cli-commands)
13. [Deployment Strategy](#deployment-strategy)
14. [Testing Strategy](#testing-strategy)
15. [Success Metrics](#success-metrics)
16. [Timeline & Milestones](#timeline--milestones)
17. [Risks & Mitigations](#risks--mitigations)
18. [Out of Scope (MVP 2)](#out-of-scope-mvp-2)

---

## Problem Statement

### Current State

**Team**: 8 developers working on Zoho SaaS applications and PHP backends for high-volume operations.

**Pain Points**:

1. **Knowledge Silos**
   - Developer A fixes a bug → solution lives in their head
   - Developer B faces the same bug 2 weeks later → re-discovers the same solution
   - No shared memory across team members
   - Architectural decisions are lost when devs are on vacation

2. **Weak QA Process**
   - Zoho is SaaS → no test runners, no automated tests
   - Manual testing is unstructured (ad-hoc, no checklist)
   - QA failures discovered in production
   - No systematic approach to edge case testing

3. **Inconsistent Code Quality**
   - Each developer has their own style
   - No code review culture
   - No linters, no formatters, no conventions enforced
   - "Cada uno programa como le da la gana"

4. **AI Adoption Chaos**
   - 8 Claude Team seats purchased
   - 5 developers have NEVER used AI coding assistants
   - 3 advanced developers use AI, but each in their own way
   - No standardized prompts, no shared practices

### Impact

- **Productivity**: 30-40% of time wasted re-discovering solutions
- **Quality**: Bugs slip to production due to weak QA
- **Onboarding**: New developers take 3-6 months to be productive
- **Technical Debt**: Inconsistent codebase is hard to maintain

---

## Vision & Goals

### Vision Statement

**"Every Conpas developer has a second brain (Jarvis) that remembers everything the team has learned, guides them through structured development, and teaches them to write better code."**

### Goals (MVP 1)

| Goal | Metric | Target |
|------|--------|--------|
| **Shared Knowledge** | % of architectural decisions documented | 90%+ |
| **Structured Development** | % of features built with SDD | 80%+ |
| **QA Coverage** | % of features with manual QA checklist | 100% |
| **Team Alignment** | Developers using same coding standards | 8/8 |
| **AI Adoption** | Developers using Jarvis daily | 6/8 (75%) |

### Non-Goals (MVP 1)

- ❌ Automated test runners (Zoho is SaaS, impossible)
- ❌ CI/CD pipelines (manual deployment is fine for now)
- ❌ Progressive onboarding (everyone starts as "intermediate")
- ❌ Advanced analytics dashboard (simple stats only)

---

## User Personas

### Persona 1: Andrés (CTO - Advanced)

**Profile**:
- 15+ years experience (GDE, MVP)
- Expert in Angular, React, PHP, Clean Architecture
- Passionate about teaching fundamentals
- Frustrated by shortcuts and "tutorial programmers"

**Needs**:
- Standardize team's development practices
- Ensure architectural decisions are documented
- Train junior developers without hand-holding
- Scale himself (not be bottleneck)

**Pain Points**:
- Repeating the same explanations to different devs
- Architectural decisions lost after 2 weeks
- No time to review every PR

**Success Scenario**: Junior dev asks "how do we handle JWT auth?" → Jarvis shows previous architectural decision + code examples → dev implements correctly without Andrés intervention.

---

### Persona 2: Carlos (Intermediate Developer)

**Profile**:
- 5 years PHP experience
- Works primarily in Zoho Creator
- Never used AI coding assistants before
- Eager to learn but overwhelmed by options

**Needs**:
- Structured guidance on how to implement features
- Clear examples of "the Conpas way"
- QA checklists to avoid bugs in production
- Confidence that he's doing things correctly

**Pain Points**:
- Doesn't know if his code follows team standards
- Afraid to ask "dumb questions" to seniors
- Manual testing is chaotic (forgets edge cases)

**Success Scenario**: Implements user bulk import feature → SDD guides him through proposal → specs → design → tasks → QA checklist → verify → done. Confident it works.

---

### Persona 3: María (Junior Developer)

**Profile**:
- 1 year experience
- Zoho focus, minimal PHP
- No AI experience
- Learns by copying examples

**Needs**:
- Step-by-step guidance
- Learn fundamentals (not just copy-paste)
- Immediate feedback when doing something wrong
- Examples of good code to reference

**Pain Points**:
- Overwhelmed by complex tasks
- Makes mistakes that seniors catch late
- Doesn't know "why" certain patterns are used

**Success Scenario**: Assigned to add field to invoice form → Jarvis breaks down into 5 small tasks → she completes each with guidance → learns CRUD fundamentals → gains confidence.

---

## Components Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        JARVIS-DEV                            │
│                  (AI Development Ecosystem)                  │
└─────────────────────────────────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
    ┌───────┐         ┌─────────┐        ┌──────────┐
    │ HIVE  │         │   SDD   │        │ PERSONA  │
    │(Memo) │         │Workflow │        │  System  │
    └───┬───┘         └────┬────┘        └────┬─────┘
        │                  │                   │
    Timeline            9 Phases           7 Presets
    Auto-sync          Manual QA          Custom
        │                  │                   │
        └──────────────────┼───────────────────┘
                           │
                           ▼
                    ┌──────────┐
                    │  SKILLS  │
                    │  System  │
                    └──────────┘
                           │
                    Zoho Deluge
                    Laravel
                    Git Workflow
```

### Agent Support

Jarvis-Dev configures whichever AI coding agents the developer has installed. The installer detects available agents and configures each one.

| Agent | Support | Config files |
|-------|---------|-------------|
| **Claude Code** | ✅ MVP 1 (default) | `~/.claude/CLAUDE.md`, `~/.claude/settings.json`, `~/.claude/skills/` |
| **OpenCode** | ✅ MVP 1 (if installed) | `~/.config/opencode/AGENTS.md`, `~/.config/opencode/opencode.json`, `~/.config/opencode/skills/` |

**Install behavior**: `jarvis install` auto-detects which agents are present and configures all of them. No flags needed — if OpenCode is installed, it gets configured automatically.

**Persona sync**: `jarvis persona set <preset>` patches the personality section in ALL configured agents simultaneously.

**Skills**: shared format, copied to each agent's skills directory.

---

## Component 1: Hive (Shared Memory)

### Overview

**What**: Persistent, searchable, shared memory system for the development team.

**Why**: Eliminate knowledge silos. Every architectural decision, bug fix, and discovery is saved automatically and available to the entire team.

**Inspiration**: Based on Engram (Gentleman-Programming), but with:
- Multi-user sync (Engram is local-only)
- Cloud central storage (PostgreSQL)
- Auto-sync (no manual commands)

### Architecture

```
┌─────────────────────────────────────────────┐
│              8 Developers                    │
│  (Andrés, Carlos, María, Pedro, etc.)       │
└─────────────────────────────────────────────┘
                    │
        ┌───────────┼───────────┐
        │           │           │
    ┌───▼────┐  ┌──▼─────┐  ┌──▼─────┐
    │ PC 1   │  │ PC 2   │  │ PC 3   │
    └───┬────┘  └───┬────┘  └───┬────┘
        │           │           │
   ┌────▼─────┐┌───▼──────┐┌───▼──────┐
   │hive-     ││hive-     ││hive-     │
   │daemon    ││daemon    ││daemon    │
   └────┬─────┘└───┬──────┘└───┬──────┘
        │          │           │
   ┌────▼─────┐┌──▼───────┐┌──▼───────┐
   │SQLite    ││SQLite    ││SQLite    │
   │(local)   ││(local)   ││(local)   │
   └────┬─────┘└───┬──────┘└───┬──────┘
        │          │           │
        └──────────┼───────────┘
                   │ (auto-sync)
        ┌──────────▼──────────┐
        │   HIVE CLOUD API    │
        │   (Go + Gin REST)   │
        └──────────┬──────────┘
                   │
        ┌──────────▼──────────┐
        │   PostgreSQL 15     │
        │   (VPS Conpas)      │
        └─────────────────────┘
```

### Features

#### 1. Hive Local (hive-daemon)

**Technology**: Go + SQLite + FTS5

**What it does**:
- MCP server (communicates with AI agents via stdio)
- Stores memories locally in SQLite (~/.jarvis/projects/{project}/memory.db)
- Full-text search with FTS5
- Offline-first (works without internet)

**Data Schema (SQLite)**:
```sql
CREATE TABLE memories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE NOT NULL,
  project TEXT NOT NULL,
  topic_key TEXT,
  category TEXT NOT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  tags TEXT,
  files_affected TEXT,
  related_to TEXT,
  created_by TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  confidence TEXT,
  impact_score INTEGER DEFAULT 0,
  synced_at DATETIME,
  origin TEXT,
  deleted_at DATETIME
);

CREATE VIRTUAL TABLE memories_fts USING fts5(
  title, content, tags,
  content='memories',
  content_rowid='id'
);
```

**MCP Tools Exposed**:
- `mem_save(title, content, type, project)` - Save a memory
- `mem_search(query, project, limit)` - Full-text search
- `mem_get_observation(id)` - Get full content
- `mem_session_summary(content, project)` - Save session summary
- `mem_context(project, limit)` - Get recent context

---

#### 2. Hive Cloud (hive-api)

**Technology**: Go + Gin + PostgreSQL 15

**What it does**:
- REST API for sync operations
- Central storage (PostgreSQL)
- Auth with JWT (email + password)
- Admin endpoints (promote user, grant admin, stats)

**Endpoints**:
```
POST   /api/v1/auth/login              → Login (email + password) → JWT
GET    /api/v1/auth/me                 → Current user info

POST   /api/v1/memories                → Create memory
GET    /api/v1/memories                → List memories (filters: project, category, date)
GET    /api/v1/memories/:id            → Get memory by ID
GET    /api/v1/memories/search         → Full-text search

POST   /api/v1/sync                    → Pull + Push memories

POST   /api/v1/admin/users/:username/level          → Promote user level
POST   /api/v1/admin/users/:username/grant-admin    → Grant admin privileges
GET    /api/v1/admin/stats                          → Team stats
```

**Database Schema (PostgreSQL)**:
```sql
CREATE TABLE memories (
  id SERIAL PRIMARY KEY,
  sync_id UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
  project VARCHAR(100) NOT NULL,
  topic_key VARCHAR(255),
  category VARCHAR(50) NOT NULL,
  title VARCHAR(500) NOT NULL,
  content TEXT NOT NULL,
  tags JSONB DEFAULT '[]',
  files_affected JSONB DEFAULT '[]',
  related_to JSONB DEFAULT '[]',
  created_by VARCHAR(100) NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  confidence VARCHAR(10),
  impact_score INTEGER DEFAULT 0,
  synced_at TIMESTAMP,
  origin VARCHAR(10),
  deleted_at TIMESTAMP,
  search_vector tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('spanish', coalesce(title,'')), 'A') ||
    setweight(to_tsvector('spanish', coalesce(content,'')), 'B')
  ) STORED
);

CREATE INDEX memories_search_idx ON memories USING GIN(search_vector);
CREATE INDEX idx_memories_project ON memories(project);
CREATE INDEX idx_memories_created_by ON memories(created_by);

CREATE TABLE users (
  username VARCHAR(100) PRIMARY KEY,
  email VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  level VARCHAR(20) DEFAULT 'intermediate',
  tasks_completed INTEGER DEFAULT 0,
  is_admin BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMP DEFAULT NOW(),
  last_active TIMESTAMP
);

CREATE TABLE projects (
  name VARCHAR(100) PRIMARY KEY,
  created_at TIMESTAMP DEFAULT NOW(),
  last_synced TIMESTAMP
);

CREATE TABLE categories (
  id VARCHAR(50) PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  description TEXT,
  icon VARCHAR(10)
);
```

---

#### 3. Auto-Sync

**When it syncs**:

1. **Session start**: Pull latest memories from team
   ```
   Usuario: cd ~/proyectos/conpas-erp
   
   hive-daemon:
     → GET /api/v1/sync?project=conpas-erp
     → Pulls 5 new memories
     → "🔄 Synced: 5 new memories from team"
   ```

2. **Memory saved**: Push immediately
   ```
   IA: mem_save(title="JWT auth implementation", ...)
   
   hive-daemon:
     → POST /api/v1/memories (background, non-blocking)
     → "✓ Memory synced to cloud"
   ```

3. **Session end**: Push pending memories
   ```
   Usuario: exits VSCode
   
   hive-daemon:
     → POST /api/v1/sync (pending memories)
     → "✓ Session closed, memories synced"
   ```

**Offline Mode**:
- If network fails → queues for retry
- Works 100% offline (local SQLite)
- Syncs when network returns

---

#### 4. Memory Categories

Config-driven categories:

```yaml
categories:
  - id: architecture
    name: Arquitectura
    description: Decisiones de arquitectura y diseño técnico
    icon: 🏗️
    
  - id: bugfix
    name: Bugs Resueltos
    description: Bugs con root cause y solución
    icon: 🐛
    
  - id: feature
    name: Features
    description: Nuevas funcionalidades implementadas
    icon: ✨
    
  - id: config
    name: Configuración
    description: Setup, env vars, deployment
    icon: ⚙️
    
  - id: discovery
    name: Descubrimientos
    description: Hallazgos no obvios, gotchas, edge cases
    icon: 🔍
    
  - id: pattern
    name: Patrones
    description: Convenciones, naming, estructura
    icon: 📐
    
  - id: decision
    name: Decisiones
    description: Decisiones de proceso/workflow (no técnicas)
    icon: 🎯
```

Easily extensible (add new categories without code changes).

---

### User Stories: Hive

**Story 1: Architectural Decision Persists**
```
As Carlos (intermediate dev)
I want to see past architectural decisions
So that I don't re-implement the same thing differently

Scenario:
  Given Andrés decided "Use JWT with RS256 for auth" 2 weeks ago
  When Carlos starts implementing user login
  Then IA searches Hive automatically
  And shows: "Architecture decision found: JWT with RS256"
  And Carlos implements it the same way
```

**Story 2: Bug Fix Shared Across Team**
```
As María (junior dev)
I want to learn from bugs fixed by others
So that I don't repeat the same mistakes

Scenario:
  Given Pedro fixed "Zoho API rate limit" bug yesterday
  When María hits the same error
  Then IA searches Hive: "zoho api rate limit"
  And shows Pedro's solution: "Use bulk operations, not loops"
  And María applies the same fix
```

**Story 3: Sync Across Devices**
```
As Andrés (CTO)
I want my memories synced between office PC and home laptop
So that I have the same context everywhere

Scenario:
  Given Andrés creates memory on office PC
  And auto-sync pushes to Hive Cloud
  When Andrés opens laptop at home
  Then auto-sync pulls latest memories
  And context is identical on both devices
```

---

## Component 2: SDD Workflow

### Overview

**What**: Spec-Driven Development - a structured methodology that forces developers to THINK before coding.

**Why**: 
- Eliminates "code first, think later" approach
- Documents decisions for future reference
- Catches design flaws before implementation
- Generates QA checklists for manual testing (critical for Zoho)

**Process**:
```
explore (optional) → proposal → specs → design → tasks → apply → qa → verify → archive
```

### The 9 Phases

#### Phase 1: sdd-explore (Optional)

**Purpose**: Investigate approaches before committing to one.

**When**: Complex problems with multiple solutions.

**Output**: Exploration document comparing options.

**Example**:
```
User: "We need bulk import of users from CSV"

AI executes: sdd-explore user-bulk-import

Output (docs/sdd/user-bulk-import/explore.md):
  # Exploration: User Bulk Import
  
  ## Option A: Zoho Deluge Script
  Pros: Native to Zoho, no external dependencies
  Cons: 5000 statement limit, no progress feedback
  
  ## Option B: PHP Backend + Zoho API
  Pros: Handle large files, better error handling
  Cons: Need API credentials, more complex
  
  ## Option C: Zoho Flow
  Pros: Visual workflow, easy for non-devs
  Cons: Limited file size, no custom validation
  
  Recommendation: Option B (PHP backend)
  Reason: Conpas has PHP expertise, scalability needed
```

---

#### Phase 2: sdd-propose

**Purpose**: Define intention, scope, and high-level approach.

**Output**: Proposal document.

**Format**:
```markdown
# Proposal: User Bulk Import

## Intent
Allow admins to import 1000+ users from CSV without manual entry.

## Scope
- Upload CSV file (columns: name, email, role, department)
- Validate format (check required fields, email format, role values)
- Preview before import (show first 10 rows)
- Import in batches (100 users/batch to avoid timeout)
- Error report (log failed rows with reason)

Out of scope:
- Edit users after import (manual process)
- Duplicate detection (must be done before CSV upload)

## Approach
- PHP Laravel backend endpoint: POST /api/users/import
- Validation: Laravel Request with custom rules
- Storage: Queue job to process CSV in background
- Zoho API: zoho.crm.bulkCreate (batch of 100)
- Response: Job ID for status polling

## Risks
- File size > 10MB might timeout
- Zoho API rate limit (100 req/min)

## Dependencies
- Zoho API credentials configured
- Redis for queue
```

---

#### Phase 3: sdd-spec

**Purpose**: Write detailed requirements and test scenarios.

**Output**: Specification document (delta spec for this change).

**Format**:
```markdown
# Spec: User Bulk Import

## Requirements

### REQ-1: File Upload
- User uploads CSV file (max 10MB)
- Accepted columns: name (required), email (required), role (required), department (optional)
- If missing columns → reject with error message
- If extra columns → ignore them

### REQ-2: Validation
- Email format: RFC 5322 compliant
- Role values: Admin, User, Guest (case-insensitive)
- Name: Max 100 chars, no special chars except space, hyphen, apostrophe
- Department: Max 50 chars (optional)

### REQ-3: Preview
- Show first 10 rows after validation
- Display: row number, name, email, role, department
- If validation errors → highlight row in red with error message

### REQ-4: Import
- Process in batches of 100 users
- Use Zoho API: zoho.crm.bulkCreate
- Progress: return Job ID for polling status
- Success: return count of imported users
- Failure: return CSV with failed rows + error reasons

### REQ-5: Error Handling
- Duplicate email in CSV → fail that row, continue with others
- Zoho API rate limit → retry with exponential backoff (1s, 2s, 4s)
- Timeout after 5 minutes → return partial results

## Scenarios

### Scenario 1: Valid CSV, 100 users
Given CSV with 100 valid users
When user uploads and clicks Import
Then all 100 users created in Zoho
And success message: "100 users imported"

### Scenario 2: Invalid email format
Given CSV with row: "John Doe, john.doe@invalid, Admin"
When validation runs
Then preview shows row in red
And error: "Invalid email format"

### Scenario 3: Large file (1500 users)
Given CSV with 1500 users
When import starts
Then processes in 15 batches (100 each)
And polling endpoint shows progress: 500/1500 imported
And completes in ~3 minutes

### Scenario 4: Zoho API rate limit
Given 500 users to import
When Zoho API hits rate limit (100 req/min)
Then import retries with backoff
And eventually completes (takes ~5 min instead of 1 min)

Total scenarios: 15 (4 shown above)
```

---

#### Phase 4: sdd-design

**Purpose**: Technical decisions - HOW to implement.

**Output**: Design document.

**Format**:
```markdown
# Design: User Bulk Import

## Architecture

```
Client (Zoho UI)
    ↓ POST /api/users/import (CSV file)
Laravel Backend
    ↓ validate CSV
    ↓ dispatch ImportUsersJob
Redis Queue
    ↓ process job
ImportUsersJob
    ↓ call Zoho API (batches of 100)
Zoho CRM
```

## File Changes

### New Files
- `app/Http/Controllers/UserImportController.php` - Handles upload
- `app/Http/Requests/ImportUsersRequest.php` - Validation rules
- `app/Jobs/ImportUsersJob.php` - Background processing
- `app/Services/ZohoBulkService.php` - Zoho API calls
- `database/migrations/2026_04_15_create_import_logs_table.php` - Log results

### Modified Files
- `routes/api.php` - Add POST /api/users/import
- `config/queue.php` - Configure Redis connection

## Key Decisions

### Decision 1: Queue vs Sync
**Choice**: Queue (Redis)
**Reason**: Large files take 3-5 min, can't block HTTP request
**Alternative rejected**: Sync processing (would timeout)

### Decision 2: Batch Size
**Choice**: 100 users/batch
**Reason**: Zoho API accepts max 100 records in bulkCreate
**Alternative rejected**: 200/batch (would fail)

### Decision 3: Error Strategy
**Choice**: Continue on row failure, log errors, return partial success
**Reason**: Better UX (import 900/1000 than fail all)
**Alternative rejected**: Fail entire import on first error (too strict)

## Data Model

```php
// import_logs table
Schema::create('import_logs', function (Blueprint $table) {
    $table->id();
    $table->string('job_id')->unique();
    $table->string('filename');
    $table->integer('total_rows');
    $table->integer('success_count')->default(0);
    $table->integer('failed_count')->default(0);
    $table->text('errors')->nullable(); // JSON array of failed rows
    $table->enum('status', ['pending', 'processing', 'completed', 'failed']);
    $table->timestamps();
});
```

## Security
- CSRF token required
- Admin role required (middleware)
- File size limit enforced (10MB)
- CSV parsing with validation (prevent CSV injection)

## Performance
- Redis queue (async processing)
- Batch processing (100 users/batch)
- Rate limit handling (exponential backoff)
- Timeout: 5 min max
```

---

#### Phase 5: sdd-tasks

**Purpose**: Break down into implementation checklist.

**Output**: Task list (markdown checklist).

**Format**:
```markdown
# Tasks: User Bulk Import

## Phase 1: Models & Migrations
- [ ] Create migration: create_import_logs_table
- [ ] Run migration: php artisan migrate

## Phase 2: Request Validation
- [ ] Create ImportUsersRequest
- [ ] Add rules: file (required, csv, max:10MB)
- [ ] Add custom validation: check CSV columns (name, email, role)

## Phase 3: Controller
- [ ] Create UserImportController
- [ ] Method: upload() - validate file, dispatch job, return job_id
- [ ] Method: status() - poll job progress

## Phase 4: Job
- [ ] Create ImportUsersJob
- [ ] Parse CSV with League\Csv
- [ ] Validate each row (email format, role values)
- [ ] Split into batches (100 users/batch)
- [ ] Call ZohoBulkService for each batch
- [ ] Update import_logs (success_count, failed_count, errors)

## Phase 5: Zoho Service
- [ ] Create ZohoBulkService
- [ ] Method: bulkCreateUsers($users) - call zoho.crm.bulkCreate
- [ ] Handle rate limit (catch exception, retry with backoff)
- [ ] Return success/failure per user

## Phase 6: Routes
- [ ] Add route: POST /api/users/import
- [ ] Add route: GET /api/users/import/{job_id}/status
- [ ] Middleware: auth, admin

## Phase 7: Tests (if PHP project with PHPUnit)
- [ ] Test: valid CSV uploads successfully
- [ ] Test: invalid CSV rejected
- [ ] Test: large file processed in batches
- [ ] Test: rate limit handled

Total: 19 tasks
```

---

#### Phase 6: sdd-apply

**Purpose**: Implement the tasks (write code).

**Output**: Code + progress log.

**How it works**:
1. AI reads tasks list
2. Implements each task sequentially
3. Updates progress: `[x] Task completed`
4. Saves progress to Hive: `sdd/user-bulk-import/apply-progress`

**Example progress**:
```markdown
# Apply Progress: User Bulk Import

## Status: In Progress (12/19 tasks completed)

## Completed Tasks
[x] Create migration: create_import_logs_table
[x] Run migration: php artisan migrate
[x] Create ImportUsersRequest
[x] Add rules: file validation
[x] Add custom validation: CSV columns
[x] Create UserImportController
[x] Method: upload() implemented
[x] Method: status() implemented
[x] Create ImportUsersJob
[x] Parse CSV with League\Csv
[x] Validate each row
[x] Split into batches

## In Progress
[ ] Call ZohoBulkService for each batch

## Pending
[ ] Update import_logs
[ ] Create ZohoBulkService
[ ] ... (7 more)

## Files Changed
- database/migrations/2026_04_15_create_import_logs_table.php (new)
- app/Http/Requests/ImportUsersRequest.php (new)
- app/Http/Controllers/UserImportController.php (new)
- app/Jobs/ImportUsersJob.php (new, partial)
- routes/api.php (modified)
```

---

#### Phase 7: sdd-qa (CRITICAL for Zoho)

**Purpose**: Generate manual QA checklist (since Zoho has no test runner).

**Output**: QA checklist (markdown).

**Format**:
```markdown
# Manual QA Checklist: User Bulk Import

**Tester**: _____________  
**Date**: _____________  
**Status**: ⬜ Pass | ⬜ Fail

---

## Test 1: Valid CSV with 10 users

**Input**: `tests/fixtures/users-valid-10.csv`
```csv
name,email,role,department
John Doe,john@conpas.dev,Admin,IT
Jane Smith,jane@conpas.dev,User,Sales
...
```

**Steps**:
1. Login to Zoho as Admin
2. Navigate to Users → Bulk Import
3. Upload `users-valid-10.csv`
4. Click "Preview"
5. Verify: 10 rows shown, all green (valid)
6. Click "Import"
7. Wait for completion (~10 seconds)

**Expected**:
- ✅ All 10 users created in Zoho
- ✅ Success message: "10 users imported successfully"
- ✅ Users appear in Users list
- ✅ Email welcome sent to each user (check inbox)

**Result**: ⬜ PASS | ⬜ FAIL  
**Notes**: _____________________________________________

---

## Test 2: Invalid email format

**Input**: `tests/fixtures/users-invalid-email.csv`
```csv
name,email,role
John Doe,john.doe@invalid,Admin
Jane Smith,jane@conpas.dev,User
```

**Steps**:
1. Upload `users-invalid-email.csv`
2. Click "Preview"

**Expected**:
- ✅ Row 1 highlighted in RED
- ✅ Error message: "Invalid email format"
- ✅ Row 2 shown in GREEN (valid)
- ✅ Import button disabled until row 1 fixed

**Result**: ⬜ PASS | ⬜ FAIL  
**Notes**: _____________________________________________

---

## Test 3: Large file (1000 users)

**Input**: `tests/fixtures/users-large-1000.csv` (generated)

**Steps**:
1. Upload file
2. Click "Import"
3. Observe progress polling

**Expected**:
- ✅ Job ID returned: "job-abc123"
- ✅ Polling shows progress: 100/1000, 200/1000, etc.
- ✅ Completes in ~2-3 minutes
- ✅ Final status: "1000 users imported successfully"
- ✅ Verify in Zoho: 1000 users exist

**Result**: ⬜ PASS | ⬜ FAIL  
**Notes**: _____________________________________________

---

## Test 4: Duplicate email in CSV

**Input**: 
```csv
name,email,role
John Doe,john@conpas.dev,Admin
Jane Smith,john@conpas.dev,User
```

**Expected**:
- ✅ Row 1 imports successfully
- ✅ Row 2 fails with error: "Duplicate email"
- ✅ Final status: "1 user imported, 1 failed"
- ✅ Download error CSV shows row 2 with reason

**Result**: ⬜ PASS | ⬜ FAIL  
**Notes**: _____________________________________________

---

## Test 5: Zoho API rate limit (edge case)

**Setup**: Import 500 users rapidly

**Expected**:
- ✅ Import handles rate limit gracefully
- ✅ No crashes or 500 errors
- ✅ Eventually completes (may take 5-6 min instead of 1 min)
- ✅ All 500 users imported

**Result**: ⬜ PASS | ⬜ FAIL  
**Notes**: _____________________________________________

---

## Test 6: Network timeout

**Setup**: Start import, disconnect network mid-process

**Expected**:
- ✅ Error message: "Network error, please retry"
- ✅ Job status: "failed"
- ✅ Partial import count shown: "450/1000 users imported before failure"
- ✅ Can retry with remaining 550 users

**Result**: ⬜ PASS | ⬜ FAIL  
**Notes**: _____________________________________________

---

## Summary

Total Tests: 6  
Passed: _____ / 6  
Failed: _____ / 6  

**Overall Status**: ⬜ PASS | ⬜ FAIL

**Sign-off**: _______________ (Tester)
```

**Flow**:
1. AI generates checklist
2. AI shows checklist in conversation
3. User executes tests in Zoho
4. User responds: "Test 1 ✅, Test 2 ✅, Test 3 ❌ (timeout at 500 users)"
5. AI parses response
6. If any FAIL → go back to `sdd-apply` with fix
7. If all PASS → continue to `sdd-verify`

**Blocker**: Cannot proceed to `verify` without QA PASS.

---

#### Phase 8: sdd-verify

**Purpose**: Validate implementation against spec, design, and tasks.

**Output**: Verification report.

**Format**:
```markdown
# Verification Report: User Bulk Import

**Date**: 2026-04-15  
**Verified By**: AI Agent  
**Verdict**: ✅ PASS (0 CRITICAL, 1 WARNING, 2 SUGGESTIONS)

---

## Compliance Check

| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| REQ-1 | File Upload | Test 1, Test 2 | ✅ COMPLIANT |
| REQ-2 | Validation | Test 2, Test 4 | ✅ COMPLIANT |
| REQ-3 | Preview | Test 2 | ✅ COMPLIANT |
| REQ-4 | Import | Test 1, Test 3 | ✅ COMPLIANT |
| REQ-5 | Error Handling | Test 4, Test 5, Test 6 | ✅ COMPLIANT |

**Total**: 5/5 requirements compliant

---

## Design Compliance

| Decision | Implementation | Result |
|----------|----------------|--------|
| Queue (Redis) | ImportUsersJob dispatched | ✅ COMPLIANT |
| Batch size 100 | ZohoBulkService splits correctly | ✅ COMPLIANT |
| Error strategy | Partial success logged | ✅ COMPLIANT |
| Rate limit | Exponential backoff implemented | ✅ COMPLIANT |

**Total**: 4/4 design decisions followed

---

## Task Completion

19/19 tasks completed ✅

---

## Issues Found

### ⚠️ WARNING

**Issue**: Import timeout hardcoded to 5 minutes  
**Impact**: Files with 2000+ users might fail  
**Recommendation**: Make timeout configurable (env var)  
**Severity**: Medium  

### 💡 SUGGESTION

**Issue**: No progress notification (user must poll manually)  
**Recommendation**: Add WebSocket for real-time progress updates  
**Severity**: Low (nice-to-have)

**Issue**: Error CSV not auto-downloaded  
**Recommendation**: Auto-download failed rows CSV after completion  
**Severity**: Low (UX improvement)

---

## Test Coverage

Manual QA: 6/6 tests passed ✅  
Edge cases covered: Rate limit, timeout, duplicates

---

## Code Quality

- [x] No hardcoded secrets
- [x] Follows Conpas naming conventions
- [x] Error messages are user-friendly
- [x] Logging implemented (import_logs table)
- [x] Security: CSRF, admin middleware, file size limit

---

## Final Verdict

✅ **PASS** - Ready for archive

**Sign-off**: AI Agent + Carlos (tester)
```

---

#### Phase 9: sdd-archive

**Purpose**: Close the change, sync artifacts, mark as done.

**Output**: Archive report + state update.

**What happens**:
1. All artifacts synced to Hive (proposal, spec, design, tasks, qa, verify)
2. Files committed to git (docs/sdd/user-bulk-import/)
3. State marked as "archived"
4. Summary saved to Hive for future reference

**Archive Report**:
```markdown
# Archive Report: User Bulk Import

**Archived**: 2026-04-15  
**Verdict**: ✅ PASS  
**Duration**: 5 days (proposal → archive)

---

## Artifacts

| Artifact | Location |
|----------|----------|
| Proposal | docs/sdd/user-bulk-import/proposal.md |
| Spec | docs/sdd/user-bulk-import/spec.md |
| Design | docs/sdd/user-bulk-import/design.md |
| Tasks | docs/sdd/user-bulk-import/tasks.md |
| QA Checklist | docs/sdd/user-bulk-import/qa-checklist.md |
| Verify Report | docs/sdd/user-bulk-import/verify-report.md |

All synced to Hive ✅

---

## Summary

**What**: Bulk import of users from CSV file into Zoho CRM

**Why**: Manual entry of 1000+ users was taking 2-3 days

**Impact**: Reduced import time from 3 days to 3 minutes

**Learnings**:
- Zoho API rate limit is 100 req/min (must use exponential backoff)
- CSV files > 1500 rows should be processed overnight (timeout risk)
- League\Csv library handles CSV parsing better than native PHP fgetcsv
- Redis queue is essential for long-running imports (don't block HTTP)

---

## Metrics

- Requirements: 5/5 compliant
- QA Tests: 6/6 passed
- Code Quality: PASS
- Duration: 5 days (under estimate of 1 week)

---

## Next Steps

None. Feature complete and deployed.
```

---

### SDD Commands (User-Facing)

**Meta-commands** (orchestrator handles):
```bash
/sdd-new <change-name>       # Start new change (explore + propose)
/sdd-continue [change-name]  # Continue to next phase
/sdd-ff [change-name]        # Fast-forward (proposal → tasks, auto)
```

**Skills** (individual phases):
```bash
/sdd-explore <topic>         # Investigate options
/sdd-apply [change-name]     # Implement tasks
/sdd-qa [change-name]        # Generate QA checklist
/sdd-verify [change-name]    # Validate implementation
/sdd-archive [change-name]   # Close change
```

---

### Integration with Hive

Every SDD phase saves to Hive:

```
Hive Topic Keys:
- sdd/{change-name}/explore
- sdd/{change-name}/proposal
- sdd/{change-name}/spec
- sdd/{change-name}/design
- sdd/{change-name}/tasks
- sdd/{change-name}/apply-progress
- sdd/{change-name}/qa-checklist
- sdd/{change-name}/verify-report
- sdd/{change-name}/archive-report
```

**Benefit**: 
- Any dev can see what was decided in each phase
- IA searches Hive when implementing similar features
- Knowledge persists beyond files (searchable)

---

### User Stories: SDD

**Story 1: Junior Dev Completes Feature with Guidance**
```
As María (junior)
I want step-by-step guidance to implement features
So that I don't get overwhelmed

Scenario:
  Given I'm assigned "Add invoice PDF export"
  When I start with /sdd-new invoice-pdf-export
  Then AI walks me through:
    - Exploration (3 PDF library options)
    - Proposal (scope, approach, timeline)
    - Spec (15 requirements, 20 test scenarios)
    - Design (file changes, library choice)
    - Tasks (12 tasks broken down)
  And I implement task by task
  And QA checklist ensures I test all edge cases
  And I complete the feature confidently
```

**Story 2: Complex Feature Planned Properly**
```
As Andrés (CTO)
I want complex features planned before coding starts
So that we don't waste time on wrong approaches

Scenario:
  Given Carlos wants to implement "Zoho-to-PHP data sync"
  When he uses /sdd-new zoho-php-sync
  Then AI forces him to:
    - Explore 3 sync strategies (polling, webhooks, queue)
    - Propose chosen approach with tradeoffs
    - Spec edge cases (network failure, partial sync, rollback)
    - Design architecture (cron job, Redis queue, error handling)
  And I review proposal before he codes anything
  And we avoid "code first, regret later"
```

**Story 3: QA Checklist Prevents Production Bugs**
```
As Carlos (intermediate)
I want a QA checklist generated automatically
So that I don't forget to test edge cases

Scenario:
  Given I finish implementing "user bulk import"
  When AI executes sdd-qa
  Then it generates 6 manual tests:
    - Valid CSV
    - Invalid email
    - Large file (1000+ users)
    - Duplicates
    - Rate limit
    - Network failure
  And I execute each test in Zoho
  And I catch the "timeout on 1500 users" bug BEFORE production
  And I fix it before archiving
```

---

## Component 3: Persona System

### Overview

**What**: 2-layer AI personality system.

**Why**: Allow developers to choose HOW the AI communicates, without breaking workflow rules.

**Layers**:
1. **Layer 1 (Base Immutable)**: Behavior, expertise, workflow rules (defined by Conpas, NOT editable)
2. **Layer 2 (User Preset)**: Tone, language, humor, verbosity (editable per user)

---

### Layer 1: Base Immutable

**File**: `config/persona-base.yaml` (users don't see this)

```yaml
behavior:
  sdd_enforcement: true
  qa_enforcement: true
  onboarding_respect: true
  complexity_check: true
  pedagogical_mode: true
  question_everything: true
  stop_on_questions: true

expertise:
  backend: [PHP, Laravel, Zoho, ERP Architecture, PostgreSQL]
  testing: [PHPUnit, Manual QA protocols]
  clean_code: [SOLID, Clean Architecture, Design Patterns, Refactoring]
  optimization: [Performance patterns, Bulk operations, Caching]

philosophy:
  - "CONCEPTOS > CÓDIGO: No toques una línea hasta entender conceptos"
  - "IA ES HERRAMIENTA: El humano dirige, IA ejecuta"
  - "FUNDAMENTOS PRIMERO: Clean Code, patrones, arquitectura antes que frameworks"
  - "NO SHORTCUTS: Aprendizaje real toma esfuerzo y tiempo"

workflow_rules:
  - "NUNCA saltar fase QA en SDD"
  - "SIEMPRE usar conventional commits"
  - "SIEMPRE guardar memorias automáticamente en Hive"
  - "NUNCA asumir, SIEMPRE verificar antes de afirmar"
  - "CUANDO PREGUNTA: PARA completamente, NO continuar con código"

teaching_approach:
  - "Rubber-duck: Pregunta para que el usuario piense"
  - "Explain WHY: No solo 'está mal', explicar técnicamente POR QUÉ"
  - "Show alternatives: Propone opciones con tradeoffs"
  - "Celebrate progress: Reconoce cuando hacen algo bien"
  - "Challenge assumptions: Cuestiona como mentor, no autoritario"

tools:
  - hive
  - sdd-workflow
  - git/gitlab (SSH)
  - manual-qa protocol

skills:
  core_skills: [sdd-*, hive]
  context_skills: [zoho-deluge, phpunit-testing, laravel-architecture, git-workflow]
  user_skills_registry: ".jarvis/skill-registry.md"
```

**This is immutable** → users cannot change.

---

### Layer 2: User Presets

**7 Presets Available**:

1. **argentino** (Gentleman)
   - Passionate mentor, Rioplatense Spanish
   - Direct, caring, energetic
   - Uses: "Ponete las pilas", "Bancá", "Locura"

2. **neutra**
   - Pure technical assistant
   - Objective, formal, no personality quirks
   - Minimal verbosity, maximum precision

3. **tony-stark**
   - Fast-paced genius, engineering metaphors
   - Direct to bluntness, excited by elegant solutions
   - Uses: "Boom. Resuelto", "Nivel Vengador", "Chatarra"

4. **yoda**
   - Jedi master, cryptic but precise
   - OSV syntax (Object-Subject-Verb): "El modelo de datos, fallar veo"
   - Minimal verbosity, philosophical

5. **sargento**
   - Commander to soldier, zero warmth
   - Orders and assessments only
   - Uses: "Negativo", "Proceda", "Misión completada"

6. **asturiano**
   - Backend architect from Asturias
   - Warm, friendly, uses Asturian expressions
   - Uses: "Meca", "Nin", "Esto tá prestoso"

7. **galleguinho**
   - Seasoned architect, Galician irony (retranca)
   - Skeptical but warm underneath
   - Uses: "Depende", "Malo será", "Marcho que teño que marchar"

8. **custom**
   - User edits template (validated to ensure Layer 1 not touched)

---

### Custom Preset Template

**File**: `~/.jarvis/persona-preset.yaml` (user edits this)

```yaml
preset: custom

tone:
  formality: neutral         # formal | informal | neutral
  directness: medium         # high | medium | low
  humor: none                # sarcastic | wholesome | none
  language: es-ES            # es-rioplatense | en-US | es-ES | es-asturiano | es-galego

communication_style:
  explain_before_doing: true
  show_alternatives: true
  challenge_assumptions: true
  use_analogies: false
  verbosity: moderate        # minimal | moderate | verbose

feedback_style:
  when_wrong: gentle         # confrontational | gentle | neutral
  when_learning: encouraging # encouraging | neutral | tough-love
  when_successful: celebratory # brief | celebratory | neutral

code_preferences:
  comment_verbosity: moderate
  show_examples: always
  rubber_duck_first: true

characteristic_phrases:
  greetings: []
  confirmations: []
  transitions: []
  sign_offs: []

notes: |
  [User writes preferences here]
```

**Validation**:
- If user tries to edit Layer 1 → rejected
- Only Layer 2 fields accepted

---

### Changing Preset

**Command**:
```bash
$ jarvis persona set argentino

✓ Preset changed: neutra → argentino
✓ Restart conversation to apply
```

**Conversational**:
```
User: "Cambiá el tono, quiero uno más directo"

AI: "Tenés varias opciones:
     1. argentino (Gentleman) - Passionate mentor
     2. tony-stark - Fast-paced genius
     3. sargento - Zero warmth, pure efficiency
     
     ¿Cuál preferís?"

User: "Tony Stark"

AI: "✓ Preset changed to tony-stark.
     Colega, ahora hablamos en modo Vengador. ¿Qué vamos a resolver?"
```

---

### User Stories: Persona

**Story 1: Dev Chooses Communication Style**
```
As Carlos
I want to choose how the AI talks to me
So that I'm comfortable with the interaction

Scenario:
  Given I prefer direct, no-nonsense communication
  When I run: jarvis persona set neutra
  Then AI responds in formal, objective style
  And no rhetorical questions or humor
  And I get straight answers without fluff
```

**Story 2: Preset Doesn't Break Workflow**
```
As María
I want to use "yoda" preset for fun
But I still need proper guidance

Scenario:
  Given I set preset to "yoda"
  When I ask AI to implement feature
  Then AI uses OSV syntax: "El módulo de facturas, crear debes"
  But STILL enforces SDD (proposal → spec → ...)
  And STILL generates QA checklist
  And Layer 1 rules are NOT affected
```

---

## Component 4: Skill System

### Overview

**What**: Context-aware code standards that auto-load based on files/commands.

**Why**: Different technologies (Zoho, Laravel, Git) have different best practices. Skills ensure the AI applies the right rules automatically.

---

### Skill Registry

**File**: `.jarvis/skill-registry.md` (per project)

```markdown
# Skill Registry — Proyecto Conpas ERP

## Core Skills (Jarvis-Dev)

- `sdd-*`: SDD workflow phases
- `hive`: Shared memory system

## Context Skills (Auto-load)

| Skill | Trigger |
|-------|---------|
| `zoho-deluge` | Archivos .dg, .ds, Zoho Creator, CRM workflows |
| `phpunit-testing` | Archivos *Test.php, comando phpunit |
| `laravel-architecture` | Estructura Laravel (app/, routes/, database/) |
| `git-workflow` | Comandos git (commit, push, pull, merge) |

## Project Skills (Custom)

| Skill | Trigger | Description |
|-------|---------|-------------|
| `conpas-erp-patterns` | Archivos en modules/ERP/ | Patrones de naming y estructura Conpas |

## Compact Rules

### zoho-deluge
- FORBIDDEN: Nested loops (for each inside for each)
- SOLUTION: Use Maps for O(n) lookups
- FORBIDDEN: API calls inside loops (invokeurl, createRecord)
- SOLUTION: Build List, then bulk operation
- ZERO HARDCODING: No tokens/passwords en código
- NULL SAFETY: Siempre ifnull() antes de .get()
- EARLY RETURNS: Guard clauses para evitar nesting

### phpunit-testing
- Un test = un concepto
- Arrange-Act-Assert pattern
- Nombres descriptivos: test_user_can_login_with_valid_credentials
- Factories para datos de prueba (UserFactory::create())

### laravel-architecture
- Controllers: thin (delegate to Services)
- Services: business logic
- Repositories: DB access (if needed)
- Requests: validation
- Jobs: async tasks (queues)

### git-workflow
- Conventional commits: feat/fix/chore/docs/refactor
- Branches: feature/, bugfix/, hotfix/
- NO force push to main
- Pull antes de push (avoid conflicts)

### conpas-erp-patterns
- Módulos en modules/{ModuleName}/
- Controllers: {Module}Controller.php
- Services: {Module}Service.php
- Repositories: {Module}Repository.php (if needed)
```

---

### Context Detection

**How it works**:

1. User works on file: `modules/Invoices/InvoicesController.php`
2. AI detects:
   - Laravel structure → loads `laravel-architecture` skill
   - modules/ERP/ → loads `conpas-erp-patterns` skill
3. AI applies rules from both skills automatically
4. User gets correct structure without asking

**Example**:
```
User: "Crea servicio para calcular totales de factura"

AI (internally):
  - Detected: modules/Invoices/ → conpas-erp-patterns
  - Rule: Services en {Module}Service.php
  
AI: "Creando InvoicesService.php..."

File created: modules/Invoices/InvoicesService.php
```php
<?php
namespace Modules\Invoices;

class InvoicesService
{
    public function calculateTotal(Invoice $invoice): float
    {
        // Business logic here
    }
}
```

Correct structure, automatic.
```

---

### User Stories: Skills

**Story 1: Zoho Deluge Rules Applied Automatically**
```
As Carlos
I want AI to follow Zoho best practices
Without me having to remember all the rules

Scenario:
  Given I'm editing invoice-calculation.dg (Deluge script)
  When I ask AI to "iterate users and call API for each"
  Then AI detects: .dg file → zoho-deluge skill
  And refuses: "FORBIDDEN: API calls inside loops"
  And suggests: "Build List of users, then bulkCreate outside loop"
  And I learn the correct pattern
```

**Story 2: Laravel Structure Enforced**
```
As María
I want to follow Laravel conventions
But I don't know them all yet

Scenario:
  Given I'm working in Laravel project
  When I ask AI to "create endpoint to update user"
  Then AI detects: Laravel structure → laravel-architecture skill
  And generates:
    - UpdateUserRequest.php (validation)
    - UserController@update (thin, delegates to service)
    - UserService::update (business logic)
  And I learn the layered approach
```

---

## Technical Stack

### Frontend (CLI)

**Language**: Go 1.23+  
**Libraries**:
- `cobra` - CLI framework
- `viper` - Configuration management
- `bubbletea` - TUI (for `jarvis timeline`)

---

### Backend (hive-daemon - Local)

**Language**: Go 1.23+  
**Database**: SQLite 3 + FTS5  
**Libraries**:
- `modernc.org/sqlite` - Pure Go SQLite driver
- MCP protocol (stdio communication)

---

### Backend (hive-api - Cloud)

**Language**: Go 1.23+  
**Framework**: Gin (REST API)  
**Database**: PostgreSQL 15+  
**Auth**: JWT (golang-jwt/jwt)  
**Libraries**:
- `gin-gonic/gin` - Web framework
- `lib/pq` - PostgreSQL driver
- `google/uuid` - UUID generation

---

### Deployment

**Containers**: Docker + docker-compose  
**Reverse Proxy**: Nginx  
**Process Manager**: systemd (for hive-daemon on local PCs)

---

### Git Integration

**SSH Keys**: Each dev uses their existing GitLab SSH key  
**No OAuth**: Git operations use native git CLI with SSH

---

## Database Schema

### Hive Local (SQLite)

```sql
-- Memories table
CREATE TABLE memories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sync_id TEXT UNIQUE NOT NULL,
  project TEXT NOT NULL,
  topic_key TEXT,
  category TEXT NOT NULL,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  tags TEXT,                    -- JSON array
  files_affected TEXT,          -- JSON array
  related_to TEXT,              -- JSON array
  created_by TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  confidence TEXT,
  impact_score INTEGER DEFAULT 0,
  synced_at DATETIME,
  origin TEXT,
  deleted_at DATETIME
);

-- FTS5 virtual table for search
CREATE VIRTUAL TABLE memories_fts USING fts5(
  title, content, tags,
  content='memories',
  content_rowid='id'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER memories_ai AFTER INSERT ON memories BEGIN
  INSERT INTO memories_fts(rowid, title, content, tags)
  VALUES (new.id, new.title, new.content, new.tags);
END;

CREATE TRIGGER memories_au AFTER UPDATE ON memories BEGIN
  UPDATE memories_fts 
  SET title = new.title, content = new.content, tags = new.tags
  WHERE rowid = new.id;
END;

CREATE TRIGGER memories_ad AFTER DELETE ON memories BEGIN
  DELETE FROM memories_fts WHERE rowid = old.id;
END;

-- Users table (for onboarding in MVP 2)
CREATE TABLE users (
  username TEXT PRIMARY KEY,
  level TEXT DEFAULT 'intermediate',
  tasks_completed INTEGER DEFAULT 0,
  joined_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Projects table
CREATE TABLE projects (
  name TEXT PRIMARY KEY,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

### Hive Cloud (PostgreSQL)

```sql
-- Memories table
CREATE TABLE memories (
  id SERIAL PRIMARY KEY,
  sync_id UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
  project VARCHAR(100) NOT NULL,
  topic_key VARCHAR(255),
  category VARCHAR(50) NOT NULL,
  title VARCHAR(500) NOT NULL,
  content TEXT NOT NULL,
  tags JSONB DEFAULT '[]',
  files_affected JSONB DEFAULT '[]',
  related_to JSONB DEFAULT '[]',
  created_by VARCHAR(100) NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  confidence VARCHAR(10),
  impact_score INTEGER DEFAULT 0,
  synced_at TIMESTAMP,
  origin VARCHAR(10),
  deleted_at TIMESTAMP,
  
  -- FTS search vector (auto-generated)
  search_vector tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('spanish', coalesce(title,'')), 'A') ||
    setweight(to_tsvector('spanish', coalesce(content,'')), 'B')
  ) STORED
);

-- Indexes
CREATE INDEX memories_search_idx ON memories USING GIN(search_vector);
CREATE INDEX idx_memories_project ON memories(project);
CREATE INDEX idx_memories_created_by ON memories(created_by);
CREATE INDEX idx_memories_topic_key ON memories(topic_key);
CREATE INDEX idx_memories_category ON memories(category);

-- Users table
CREATE TABLE users (
  username VARCHAR(100) PRIMARY KEY,
  email VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  level VARCHAR(20) DEFAULT 'intermediate',
  tasks_completed INTEGER DEFAULT 0,
  is_admin BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMP DEFAULT NOW(),
  last_active TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);

-- Projects table
CREATE TABLE projects (
  name VARCHAR(100) PRIMARY KEY,
  created_at TIMESTAMP DEFAULT NOW(),
  last_synced TIMESTAMP
);

-- Categories table (config-driven)
CREATE TABLE categories (
  id VARCHAR(50) PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  description TEXT,
  icon VARCHAR(10)
);

-- Seed categories
INSERT INTO categories (id, name, description, icon) VALUES
  ('architecture', 'Arquitectura', 'Decisiones de arquitectura y diseño técnico', '🏗️'),
  ('bugfix', 'Bugs Resueltos', 'Bugs con root cause y solución', '🐛'),
  ('feature', 'Features', 'Nuevas funcionalidades implementadas', '✨'),
  ('config', 'Configuración', 'Setup, env vars, deployment', '⚙️'),
  ('discovery', 'Descubrimientos', 'Hallazgos no obvios, gotchas, edge cases', '🔍'),
  ('pattern', 'Patrones', 'Convenciones, naming, estructura', '📐'),
  ('decision', 'Decisiones', 'Decisiones de proceso/workflow (no técnicas)', '🎯');
```

---

## API Specification

**Base URL**: `https://hive.conpas.dev/api/v1`  
**Auth**: Bearer JWT token

### Auth Endpoints

#### POST /auth/login
```json
Request:
{
  "email": "andres@conpas.dev",
  "password": "********"
}

Response (200):
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "username": "andres",
    "email": "andres@conpas.dev",
    "level": "intermediate",
    "is_admin": true
  }
}

Error (401):
{
  "error": "Invalid credentials"
}
```

#### GET /auth/me
```
Headers: Authorization: Bearer {token}

Response (200):
{
  "username": "andres",
  "email": "andres@conpas.dev",
  "level": "intermediate",
  "is_admin": true,
  "tasks_completed": 45
}
```

---

### Memory Endpoints

#### POST /memories
```json
Headers: Authorization: Bearer {token}

Request:
{
  "project": "conpas-erp",
  "topic_key": "architecture/auth/jwt",
  "category": "architecture",
  "title": "JWT authentication with RS256",
  "content": "**What**: Implemented JWT...",
  "tags": ["auth", "jwt", "security"],
  "files_affected": ["src/auth/jwt.ts", "config/auth.php"]
}

Response (201):
{
  "id": 123,
  "sync_id": "550e8400-e29b-41d4-a716-446655440000",
  "project": "conpas-erp",
  "title": "JWT authentication with RS256",
  "created_by": "andres",
  "created_at": "2026-04-10T10:30:00Z"
}
```

#### GET /memories
```
Headers: Authorization: Bearer {token}
Query params:
  - project (optional): Filter by project
  - category (optional): Filter by category
  - limit (optional, default 50): Max results
  - offset (optional, default 0): Pagination

Response (200):
{
  "total": 150,
  "memories": [
    {
      "id": 123,
      "project": "conpas-erp",
      "category": "architecture",
      "title": "JWT authentication",
      "created_by": "andres",
      "created_at": "2026-04-10T10:30:00Z"
    },
    ...
  ]
}
```

#### GET /memories/:id
```
Headers: Authorization: Bearer {token}

Response (200):
{
  "id": 123,
  "sync_id": "550e8400-...",
  "project": "conpas-erp",
  "topic_key": "architecture/auth/jwt",
  "category": "architecture",
  "title": "JWT authentication with RS256",
  "content": "**What**: Implemented JWT with RS256...",
  "tags": ["auth", "jwt"],
  "files_affected": ["src/auth/jwt.ts"],
  "created_by": "andres",
  "created_at": "2026-04-10T10:30:00Z"
}

Error (404):
{
  "error": "Memory not found"
}
```

#### GET /memories/search
```
Headers: Authorization: Bearer {token}
Query params:
  - query (required): Search query
  - project (optional): Filter by project
  - limit (optional, default 20): Max results

Response (200):
{
  "results": [
    {
      "id": 123,
      "title": "JWT authentication",
      "content": "...",
      "rank": 0.9523,
      "project": "conpas-erp"
    },
    ...
  ]
}
```

---

### Sync Endpoint

#### POST /sync
```json
Headers: Authorization: Bearer {token}

Request:
{
  "project": "conpas-erp",
  "memories": [
    {
      "sync_id": "550e8400-...",
      "title": "New memory",
      "content": "...",
      "category": "feature",
      "created_by": "carlos",
      "created_at": "2026-04-10T14:00:00Z"
    }
  ]
}

Response (200):
{
  "pulled": [
    {
      "id": 124,
      "sync_id": "660e8400-...",
      "title": "Memory from another dev",
      "created_by": "maria"
    }
  ],
  "pushed": 1,
  "total": 1
}
```

---

### Admin Endpoints

#### POST /admin/users/:username/level
```json
Headers: Authorization: Bearer {admin_token}

Request:
{
  "level": "advanced"
}

Response (200):
{
  "username": "carlos",
  "level": "advanced",
  "promoted_by": "andres",
  "promoted_at": "2026-04-10T15:00:00Z"
}

Error (403):
{
  "error": "Forbidden: Admin privileges required"
}
```

#### POST /admin/users/:username/grant-admin
```
Headers: Authorization: Bearer {admin_token}

Response (200):
{
  "username": "laura",
  "is_admin": true,
  "granted_by": "andres"
}

Error (400):
{
  "error": "Max admin limit reached (3/3)"
}
```

#### GET /admin/stats
```
Headers: Authorization: Bearer {admin_token}

Response (200):
{
  "total_users": 8,
  "total_memories": 450,
  "total_projects": 3,
  "users_by_level": {
    "beginner": 0,
    "intermediate": 5,
    "advanced": 3
  },
  "memories_by_category": {
    "architecture": 120,
    "bugfix": 180,
    "feature": 100,
    "other": 50
  }
}
```

---

## CLI Commands

### Auto project detection (no user action required)

**Purpose**: Automatically scope Hive memories to the correct project on every AI session — without requiring the user to run any command.

**How it works**:

The AI detects the active project at the start of every session:
1. `git remote get-url origin` → canonical project name (e.g. `conpas-erp`)
2. Fallback: current directory name
3. Fallback: `"default"` if no git repo and no meaningful dirname

This project identifier is used as the `project` field in every `mem_save` call automatically. If the project was never seen before, it is silently registered on first save — no separate init step needed.

> **Design decision**: Separating project registration (automatic, no files) from project scaffold (explicit `jarvis init`) prevents silent memory mis-scoping when the user forgets to run a command. The global hive-daemon `~/.jarvis/memory.db` already scopes memories via the `project` field — no per-project SQLite file is needed.

---

### jarvis init

**Purpose**: Scaffold the `.jarvis/` project directory — a **team artifact** committed to the repo that enables project-specific skills and config.

**Usage**:
```bash
$ cd ~/proyectos/conpas-erp
$ jarvis init

Detecting project...
✓ Project: conpas-erp (from git remote)
✓ Stack: Laravel 10, PHP 8.2

Scaffolding .jarvis/...
✓ Skill registry created: .jarvis/skill-registry.md

Skills available:
  - Core: sdd-*, hive
  - Context: zoho-deluge, phpunit-testing, laravel-architecture, git-workflow

✓ Project initialized — commit .jarvis/ to share with your team
```

**What it does**:
1. Detects project name (git remote or folder name)
2. Detects stack (Laravel, Zoho Deluge, etc.) via file heuristics
3. Creates `.jarvis/skill-registry.md` with suggested skills for the detected stack
4. Idempotent — safe to re-run (updates suggestions without overwriting custom entries)

**What it does NOT do**:
- Does NOT create a per-project SQLite database (global hive-daemon at `~/.jarvis/memory.db` handles all projects via the `project` field)
- Does NOT register the project in Hive (auto-detection at session start handles that)

---

### jarvis sync

**Purpose**: Manual sync with Hive Cloud (auto-sync normally handles this)

**Usage**:
```bash
$ jarvis sync

🔄 Syncing with Hive Cloud...
↓ Pulled: 5 memories
↑ Pushed: 3 memories
✓ Sync completed
```

---

### jarvis timeline

**Purpose**: View memory timeline (TUI)

**Usage**:
```bash
$ jarvis timeline

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Timeline: conpas-erp (150 memories)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[2026-04-10 14:30] 🏗️  architecture/auth/jwt
  JWT authentication with RS256
  
[2026-04-10 12:15] 🐛 bugfix/zoho-api/rate-limit
  Fix rate limit handling in Zoho API
  
[2026-04-09 16:00] ✨ feature/users/bulk-import
  Bulk user import from CSV

[↑↓] Navigate  [Enter] View  [Q] Quit
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

### jarvis persona set

**Purpose**: Change AI personality preset

**Usage**:
```bash
$ jarvis persona set argentino

✓ Preset changed: neutra → argentino
✓ Restart conversation to apply changes

$ jarvis persona set custom

✓ Opening custom preset for editing...
(Opens ~/.jarvis/persona-preset.yaml in $EDITOR)
```

---

### jarvis login

**Purpose**: Authenticate with Hive Cloud

**Usage**:
```bash
$ jarvis login

Email: andres@conpas.dev
Password: ********

✓ Authenticated
✓ Token saved

User: andres (Admin, Advanced)
```

---

### jarvis config

**Purpose**: View/edit configuration

**Usage**:
```bash
$ jarvis config

Current configuration:
  - Auto-sync: enabled
  - API endpoint: https://hive.conpas.dev/api/v1
  - Preset: argentino

$ jarvis config set sync.auto false

✓ Auto-sync disabled
```

---

## Deployment Strategy

### Architecture

```
VPS Conpas (Ubuntu 22.04)
├─ Docker (containerized services)
│  ├─ hive-api (Go binary in Alpine container)
│  ├─ postgres (PostgreSQL 15)
│  ├─ redis (for future queues)
│  └─ nginx (reverse proxy + SSL)
└─ Backups (cron job → daily PostgreSQL dump)
```

---

### docker-compose.yml

```yaml
version: '3.8'

services:
  hive-api:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: hive-api
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgresql://hive:${DB_PASSWORD}@postgres:5432/hive
      JWT_SECRET: ${JWT_SECRET}
    depends_on:
      - postgres
    restart: unless-stopped

  postgres:
    image: postgres:15-alpine
    container_name: hive-postgres
    environment:
      POSTGRES_DB: hive
      POSTGRES_USER: hive
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    restart: unless-stopped

  nginx:
    image: nginx:alpine
    container_name: hive-nginx
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./ssl:/etc/nginx/ssl:ro
    depends_on:
      - hive-api
    restart: unless-stopped

volumes:
  postgres_data:
```

---

### Dockerfile (hive-api)

```dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o hive-api ./cmd/hive-api

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/hive-api .
EXPOSE 8080
CMD ["./hive-api"]
```

---

### Installation Script (VPS)

```bash
#!/bin/bash
# setup-hive-cloud.sh

set -e

echo "======================================"
echo "  Hive Cloud - VPS Installation"
echo "======================================"

# 1. Install Docker
if ! command -v docker &> /dev/null; then
    curl -fsSL https://get.docker.com -o get-docker.sh
    sudo sh get-docker.sh
    sudo usermod -aG docker $USER
fi

# 2. Clone repo
sudo git clone https://gitlab.conpas.dev/conpas/hive-cloud.git /opt/hive-cloud
cd /opt/hive-cloud

# 3. Configure .env
cp .env.example .env
# ... (generate secrets, edit .env)

# 4. Start services
docker-compose up -d

# 5. Run migrations
docker-compose exec -T hive-api ./hive-api migrate

# 6. Create admin
read -p "Admin email: " ADMIN_EMAIL
docker-compose exec -T hive-api ./hive-api admin create $ADMIN_EMAIL

echo "✓ Hive Cloud installed and running"
```

---

### Local Installation (Developer PC)

```bash
# macOS
$ brew install conpasdevs/tap/jarvis-dev

# Linux
$ curl -fsSL https://jarvis.conpas.dev/install.sh | bash

# Windows
$ scoop bucket add conpasdevs https://github.com/ConpasDevs/scoop-bucket
$ scoop install jarvis-dev
```

After installing the binary, run the setup wizard:

```bash
$ jarvis install

Detecting installed AI agents...
  ✓ Claude Code found (~/.claude/)
  ✓ OpenCode found (~/.config/opencode/)

Configuring Claude Code...
  ✓ ~/.claude/CLAUDE.md written (Layer 1 base + neutra preset)
  ✓ ~/.claude/settings.json updated (hive-daemon MCP added)
  ✓ ~/.claude/skills/ populated (sdd-*, zoho-deluge, laravel-architecture, git-workflow)

Configuring OpenCode...
  ✓ ~/.config/opencode/AGENTS.md written
  ✓ ~/.config/opencode/opencode.json updated (hive-daemon MCP + SDD agents added)
  ✓ ~/.config/opencode/skills/ populated

Installing hive-daemon...
  ✓ Binary installed: ~/go/bin/hive-daemon
  ✓ systemd service enabled (auto-start on login)
  ✓ Config created: ~/.jarvis/config.yaml

Next: run 'jarvis login' to connect to Hive Cloud
```

**What it installs**:
1. `jarvis` CLI binary
2. `hive-daemon` (systemd service, auto-start)
3. Config files in `~/.jarvis/`
4. Agent configs for each detected AI agent (Claude Code, OpenCode)
5. Skills copied to each agent's skills directory

---

## Testing Strategy

### Unit Tests

**Hive Local (Go)**:
```go
func TestAddMemory(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    
    memory := &Memory{
        Project: "test",
        Title: "Test memory",
        Content: "Test content",
    }
    
    id, err := db.AddMemory(memory)
    assert.NoError(t, err)
    assert.Greater(t, id, int64(0))
}
```

**Coverage Target**: 80%+

---

### Integration Tests

**Hive Cloud API (Go)**:
```go
func TestSyncEndpoint(t *testing.T) {
    api := setupTestAPI(t)
    
    resp := api.POST("/api/v1/sync", SyncRequest{
        Project: "test",
        Memories: []Memory{{Title: "New"}},
    }, withAuth("test-user"))
    
    assert.Equal(t, 200, resp.StatusCode)
    assert.Greater(t, resp.Body.Pushed, 0)
}
```

**Coverage Target**: 85%+

---

### E2E Tests

**SDD Complete Workflow**:
```go
func TestSDD_CompleteFlow(t *testing.T) {
    project := setupTestProject(t)
    
    // 1. Init SDD
    runCommand(t, "jarvis init")
    
    // 2. Create change
    runCommand(t, "/sdd-new user-auth")
    
    // 3. Fast-forward
    runCommand(t, "/sdd-ff user-auth")
    
    // 4. Verify artifacts exist
    assert.FileExists(t, "docs/sdd/user-auth/proposal.md")
    assert.FileExists(t, "docs/sdd/user-auth/spec.md")
    
    // 5. Apply + QA + Verify
    runCommand(t, "/sdd-apply user-auth")
    simulateQA(t, "user-auth", allPass=true)
    runCommand(t, "/sdd-verify user-auth")
    
    // 6. Archive
    runCommand(t, "/sdd-archive user-auth")
    
    // 7. Verify Hive has all artifacts
    memories := searchHive(t, "sdd/user-auth")
    assert.GreaterOrEqual(t, len(memories), 7)
}
```

**Coverage Target**: 75%+ (key workflows)

---

## Success Metrics

### Quantitative Metrics

| Metric | Baseline (Now) | Target (3 months after launch) |
|--------|----------------|--------------------------------|
| **Shared Decisions Documented** | 0% | 90%+ |
| **Features with SDD** | 0% | 80%+ |
| **Features with QA Checklist** | ~30% (ad-hoc) | 100% |
| **Production Bugs** | ~10/month | <5/month |
| **Time to Onboard New Dev** | 3-6 months | 1-2 months |
| **AI Tool Adoption** | 3/8 devs (37%) | 6/8 devs (75%) |
| **Code Review Time** | ~2 hours/PR | <30 min/PR |

---

### Qualitative Metrics

| Metric | How to Measure |
|--------|----------------|
| **Developer Satisfaction** | Survey (1-10): "Jarvis helps me code better" |
| **Knowledge Retention** | Devs can find past decisions in <2 min (vs "ask Andrés") |
| **Code Quality** | Senior devs report: "Less time fixing junior mistakes" |
| **Confidence** | Junior devs: "I feel confident implementing features alone" |

---

## Timeline & Milestones

### Month 1: Hive Foundation

**Week 1-2: Hive Local (daemon)**
- [ ] SQLite schema + FTS5
- [ ] MCP server (stdio)
- [ ] Tools: mem_save, mem_search, mem_get_observation
- [ ] Auto-sync on session start/end

**Week 3-4: Hive Cloud (API)**
- [ ] PostgreSQL schema + FTS
- [ ] REST endpoints (auth, memories, sync)
- [ ] JWT authentication
- [ ] Docker deployment

**Milestone 1**: Devs can save/search memories locally + sync to cloud

---

### Month 2: SDD Core

**Week 5-6: SDD Phases 1-4**
- [ ] sdd-explore
- [ ] sdd-propose
- [ ] sdd-spec
- [ ] sdd-design

**Week 7-8: SDD Phases 5-9**
- [ ] sdd-tasks
- [ ] sdd-apply
- [ ] sdd-qa (Manual QA Protocol)
- [ ] sdd-verify
- [ ] sdd-archive

**Milestone 2**: Complete SDD workflow functional (explore → archive)

---

### Month 3: Persona + Skills

**Week 9-10: Persona System**
- [ ] Layer 1 (base immutable)
- [ ] 7 presets loaded
- [ ] Custom template validation
- [ ] `jarvis persona set` command

**Week 11-12: Skill System**
- [ ] Skill registry format
- [ ] Context detection (file extensions, commands)
- [ ] Core skills: zoho-deluge, laravel-architecture, git-workflow
- [ ] Auto-loading based on context

**Milestone 3**: Devs can choose persona + skills auto-load

---

### Month 4: CLI + Integration

**Week 13-14: CLI Commands**
- [ ] jarvis init
- [ ] jarvis sync (manual)
- [ ] jarvis timeline (TUI)
- [ ] jarvis login
- [ ] jarvis persona set
- [ ] jarvis config

**Week 15-16: Integration Tests**
- [ ] SDD complete workflow E2E
- [ ] Hive sync E2E
- [ ] Multi-user scenario tests
- [ ] Edge case testing (network failure, conflicts)

**Milestone 4**: CLI fully functional, integration tests passing

---

### Month 5: Documentation + Polish

**Week 17-18: Documentation**
- [ ] Architecture docs
- [ ] API reference
- [ ] Deployment guide
- [ ] Developer onboarding guide
- [ ] Troubleshooting guide

**Week 19-20: Alpha Testing**
- [ ] Deploy to VPS
- [ ] Install on 3 alpha testers (Andrés, Carlos, Laura)
- [ ] Collect feedback
- [ ] Fix critical bugs
- [ ] Performance tuning

**Milestone 5**: MVP 1 ready for team rollout

---

## Risks & Mitigations

### Risk 1: Adoption Resistance (HIGH)

**Risk**: 5 devs who never used AI might resist learning Jarvis-Dev.

**Impact**: Project fails if team doesn't adopt.

**Mitigation**:
- Start with 3 alpha testers (Andrés + 2 eager devs)
- Showcase quick wins (memory sharing, QA checklists)
- Mandate SDD for all new features (enforce through code review)
- Weekly training sessions (30 min, show real examples)
- Gamification: Leaderboard of SDD completions (friendly competition)

---

### Risk 2: Technical Complexity (MEDIUM)

**Risk**: Go + PostgreSQL + Docker is complex for PHP team to maintain.

**Impact**: If CTO leaves, team can't maintain system.

**Mitigation**:
- EXTREME documentation (every decision explained)
- Code heavily commented (explain WHY, not just WHAT)
- Training: 2 senior devs learn Go basics (1 month)
- Pair programming sessions (CTO + senior dev)
- Fallback: Migrate to Laravel API in MVP 2 if needed

---

### Risk 3: VPS Downtime (MEDIUM)

**Risk**: Hive Cloud VPS crashes → team can't sync.

**Impact**: Temporary inability to share memories.

**Mitigation**:
- Offline-first design (SQLite local works without cloud)
- Auto-retry sync when network returns
- Backups: Daily PostgreSQL dumps to S3/Wasabi
- Monitoring: UptimeRobot alerts on downtime
- Recovery: VPS restore < 1 hour (Docker makes it easy)

---

### Risk 4: Performance Issues (LOW)

**Risk**: PostgreSQL FTS is slow with 10,000+ memories.

**Impact**: Search latency > 1 second (bad UX).

**Mitigation**:
- GIN index on search_vector (already planned)
- Pagination (max 50 results per query)
- Caching: Redis layer for common searches (future)
- Monitoring: Log slow queries (>500ms)
- Upgrade path: Migrate to ElasticSearch if needed (MVP 3)

---

### Risk 5: QA Checklist Ignored (HIGH)

**Risk**: Devs skip manual QA, check all boxes without testing.

**Impact**: Bugs slip to production, defeats the purpose.

**Mitigation**:
- SDD blocks archive until QA signed-off
- Code review: Reviewer asks "Did you run QA checklist?"
- Random spot checks: CTO occasionally re-runs QA on archived features
- Metrics: Track "QA pass rate" (if <60%, investigate)
- Culture: Celebrate devs who catch bugs in QA (public recognition)

---

## Out of Scope (MVP 2)

### Features Deferred to MVP 2

1. **Onboarding Progresivo**
   - 3 niveles automáticos (Beginner/Intermediate/Advanced)
   - Bloqueo pedagógico
   - Tracking de progreso
   - Stats por usuario

2. **Admin Dashboard**
   - Web UI para ver stats del equipo
   - Gráficos de adopción
   - Timeline de memorias
   - Gestión de usuarios visual

3. **Skill System Avanzado**
   - User skills custom (además de core skills)
   - Skill versioning
   - Skill marketplace (compartir entre proyectos)

4. **Analytics Avanzados**
   - Métricas de productividad (time to complete SDD)
   - Análisis de código (code quality trends)
   - Predicción de bugs (ML-based)

5. **CI/CD Integration**
   - Auto-run QA checklist (cuando sea posible)
   - Deploy automation
   - Rollback automático on failure

6. **hive-api como Remote MCP**
   - hive-api expone el protocolo MCP via HTTP/SSE (como context7)
   - Claude Code y OpenCode se conectan directamente a `https://hivemem.dev/mcp`
   - Acceso a memorias del equipo en tiempo real, sin esperar sync
   - Configuración: `jarvis install` añade el MCP remoto junto al daemon local
   - Caso de uso: María guarda una memory → Carlos la ve al instante, sin `mem_sync`
   - Requiere: implementar MCP transport (SSE) en hive-api + auth por JWT

---

## Appendix A: Glossary

| Term | Definition |
|------|------------|
| **Hive** | Shared memory system (local SQLite + cloud PostgreSQL) |
| **SDD** | Spec-Driven Development (think first, code later) |
| **Persona** | AI personality layer (tone, language, verbosity) |
| **Skill** | Code standard that auto-loads based on context |
| **Topic Key** | Hierarchical identifier for memories (e.g., `architecture/auth/jwt`) |
| **Memory** | Stored observation (decision, bugfix, pattern, etc.) |
| **Category** | Classification of memory (architecture, bugfix, feature, etc.) |
| **Sync** | Process of pushing/pulling memories to/from cloud |
| **QA Checklist** | Manual test scenarios generated by AI (for Zoho) |
| **Archive** | Final phase of SDD (close change, mark as done) |

---

## Appendix B: References

- **Engram**: https://github.com/Gentleman-Programming/engram (memory system inspiration)
- **Gentle-AI**: https://github.com/Gentleman-Programming/gentle-ai (SDD workflow reference)
- **MCP Protocol**: https://modelcontextprotocol.io (AI-agent communication)
- **PostgreSQL FTS**: https://www.postgresql.org/docs/current/textsearch.html (full-text search)
- **Gin Framework**: https://gin-gonic.com (Go web framework)

---

**END OF PRD**

---

**Next Steps**:
1. Review PRD with CEO + team
2. Get approval for resources (time, VPS, Claude seats)
3. Start development (Month 1: Hive)
4. Weekly progress updates to CEO
5. Alpha test with 3 devs (Month 5)
6. Team rollout (Month 6)

**Questions? Reach CTO: andres@conpas.dev**
