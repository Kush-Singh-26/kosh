# Subjective Review Execution Instructions

**Goal:** Complete 20 subjective review batches to unlock score improvements from 84.4 → 95.0

**Status:** Mechanical improvements complete (77+ tests added, architecture verified). Score is subjective-review locked.

---

## Problem Summary

The automated codex runner failed on all 20 batches because prompts exceed Windows command line length limits. We need to execute these reviews manually using Claude (or similar AI assistant) and import the results.

---

## Option 1: Execute All 20 Batches (Recommended)

### Step 1: Launch Claude with Review Packet

Open the Claude launch prompt file:

```bash
type C:\Users\KIIT0001\blogs\.desloppify\external_review_sessions\ext_20260315_152913_6536bd28\claude_launch_prompt.md
```

**OR** read it directly in your editor:
- File: `.desloppify/external_review_sessions/ext_20260315_152913_6536bd28/claude_launch_prompt.md`

### Step 2: Copy Entire Prompt to Claude

1. Open Claude (claude.ai or Claude desktop app)
2. Copy the **entire contents** of `claude_launch_prompt.md`
3. Paste into Claude as a single message
4. Wait for Claude to complete the analysis (may take 5-10 minutes)

### Step 3: Save Claude's Output

Claude should output JSON. Save it to:

```
C:\Users\KIIT0001\blogs\.desloppify\external_review_sessions\ext_20260315_152913_6536bd28\review_result.json
```

**Important:** The output must be **valid JSON only** - no markdown code fences, no explanatory text outside the JSON structure.

### Step 4: Import Results and Rescan

```bash
cd C:\Users\KIIT0001\blogs
python -m desloppify review --external-submit --session-id ext_20260315_152913_6536bd28 --import .desloppify\external_review_sessions\ext_20260315_152913_6536bd28\review_result.json --scan-after-import
```

### Step 5: Check New Score

```bash
python -m desloppify status
```

Expected: Score should move from 84.4 toward 95.0 (exact improvement depends on review findings)

---

## Option 2: Execute Individual Batches (Alternative)

If the full packet is too large, execute batches individually.

### Batch List

| # | Dimension | Prompt File |
|---|-----------|-------------|
| 1 | cross_module_architecture | `.desloppify/subagents/runs/20260315_152418/prompts/batch-1.md` |
| 2 | design_coherence | `.desloppify/subagents/runs/20260315_152418/prompts/batch-2.md` |
| 3 | high_level_elegance | `.desloppify/subagents/runs/20260315_152418/prompts/batch-3.md` |
| 4 | mid_level_elegance | `.desloppify/subagents/runs/20260315_152418/prompts/batch-4.md` |
| 5 | test_strategy | `.desloppify/subagents/runs/20260315_152418/prompts/batch-5.md` |
| 6 | test_health | `.desloppify/subagents/runs/20260315_152418/prompts/batch-6.md` |
| 7 | api_coherence | `.desloppify/subagents/runs/20260315_152418/prompts/batch-7.md` |
| 8 | convention_drift | `.desloppify/subagents/runs/20260315_152418/prompts/batch-8.md` |
| 9 | abstraction_fit | `.desloppify/subagents/runs/20260315_152418/prompts/batch-9.md` |
| 10 | contracts | `.desloppify/subagents/runs/20260315_152418/prompts/batch-10.md` |
| 11 | error_consistency | `.desloppify/subagents/runs/20260315_152418/prompts/batch-11.md` |
| 12 | naming_quality | `.desloppify/subagents/runs/20260315_152418/prompts/batch-12.md` |
| 13 | code_quality | `.desloppify/subagents/runs/20260315_152418/prompts/batch-13.md` |
| 14 | duplication | `.desloppify/subagents/runs/20260315_152418/prompts/batch-14.md` |
| 15 | structure_nav | `.desloppify/subagents/runs/20260315_152418/prompts/batch-15.md` |
| 16 | dep_health | `.desloppify/subagents/runs/20260315_152418/prompts/batch-16.md` |
| 17 | init_coupling | `.desloppify/subagents/runs/20260315_152418/prompts/batch-17.md` |
| 18 | stale_migration | `.desloppify/subagents/runs/20260315_152418/prompts/batch-18.md` |
| 19 | auth_consistency | `.desloppify/subagents/runs/20260315_152418/prompts/batch-19.md` |
| 20 | security | `.desloppify/subagents/runs/20260315_152418/prompts/batch-20.md` |

### For Each Batch:

1. **Read the prompt:**
   ```bash
   type .desloppify\subagents\runs\20260315_152418\prompts\batch-N.md
   ```

2. **Paste into Claude** and request JSON output

3. **Save output** to:
   ```
   .desloppify\subagents\runs\20260315_152418\results\batch-N.raw.txt
   ```

4. **After all 20 batches complete**, import:
   ```bash
   python -m desloppify review --import-run .desloppify\subagents\runs\20260315_152418 --scan-after-import
   ```

---

## Expected Output Format

Claude must output **valid JSON** matching this schema:

```json
{
  "batch": "cross_module_architecture",
  "batch_index": 1,
  "assessments": {
    "cross_module_architecture": 72.5
  },
  "dimension_notes": {
    "cross_module_architecture": {
      "evidence": [
        "Specific code observation 1",
        "Specific code observation 2"
      ],
      "impact_scope": "module",
      "fix_scope": "multi_file_refactor",
      "confidence": "high",
      "issues_preventing_higher_score": "List specific issues"
    }
  },
  "dimension_judgment": {
    "cross_module_architecture": {
      "dimension_character": "2-3 sentences synthesizing positives and defects",
      "score_rationale": "2-3 sentences explaining score with global anchors"
    }
  },
  "issues": [
    {
      "dimension": "cross_module_architecture",
      "identifier": "short_unique_id",
      "summary": "One-line defect summary",
      "related_files": ["builder/cache/types.go"],
      "evidence": ["Specific code observation"],
      "suggestion": "Concrete fix recommendation",
      "confidence": "high",
      "impact_scope": "module",
      "fix_scope": "multi_file_refactor"
    }
  ],
  "context_updates": {
    "cross_module_architecture": {
      "add": [
        {
          "header": "Short label",
          "description": "Why this is the way it is",
          "settled": false,
          "positive": true
        }
      ]
    }
  }
}
```

### Critical Requirements:

1. **No markdown fences** - Raw JSON only, no ```json blocks
2. **No explanatory text** outside the JSON structure
3. **Valid JSON syntax** - Proper quotes, commas, brackets
4. **All required fields** present (batch, batch_index, assessments, dimension_judgment, issues)
5. **Scores as decimals** - e.g., 72.5 not 72

---

## Verification After Import

After importing, verify:

```bash
# Check review queue
python -m desloppify show review --status open

# Check score
python -m desloppify status

# Check subjective dimensions
python -m desloppify show subjective
```

Expected improvements:
- **Test strategy:** 68.5% → 85%+ (we added 77+ tests)
- **Test health:** Should recognize new integration tests
- **Mid elegance:** 78.5% → 85%+ (Phase 2 fixes)
- **Cross-module arch:** 72.5% → 85%+ (verified decoupling)
- **Overall strict:** 84.4 → 90-95 range

---

## Troubleshooting

### If Claude Output is Invalid JSON

**Error:** `desloppify review --import` fails with JSON parse error

**Fix:**
1. Remove markdown fences (```json ... ```)
2. Remove any text before/after JSON
3. Validate JSON at https://jsonlint.com/
4. Re-import

### If Import Succeeds But Score Doesn't Move

**Possible causes:**
1. Review didn't find defects (code is actually good)
2. Scores were already accurate
3. Need to resolve open issues after import

**Fix:**
```bash
# See what issues were created
python -m desloppify show review --status open

# Resolve issues you've already fixed
python -m desloppify plan resolve <issue-id>

# Rescan to measure
python -m desloppify scan
```

### If Claude Refuses or Times Out

**Alternative:** Split into smaller chunks
- Send 5 batches at a time
- Or use the individual batch approach (Option 2)

---

## Report Back Format

After execution, please provide:

1. **New strict score:** `__ / 100`
2. **New overall score:** `__ / 100`
3. **Issues created:** `__ new issues`
4. **Issues resolved:** `__ issues auto-resolved`
5. **Biggest dimension improvements:**
   - Dimension: `__%` → `__%` (+`__`)
   - Dimension: `__%` → `__%` (+`__`)
6. **Any errors encountered:** [description]
7. **Output file location:** [path to review_result.json]

---

## Files Reference

| File | Purpose |
|------|---------|
| `.desloppify/external_review_sessions/ext_20260315_152913_6536bd28/claude_launch_prompt.md` | Full review prompt (all 20 batches) |
| `.desloppify/external_review_sessions/ext_20260315_152913_6536bd28/review_result.json` | **Output target** - Save Claude's JSON here |
| `.desloppify/external_review_sessions/ext_20260315_152913_6536bd28/reviewer_instructions.md` | Detailed reviewer guidelines |
| `.desloppify/subagents/runs/20260315_152418/prompts/batch-N.md` | Individual batch prompts (N=1-20) |
| `.desloppify/subagents/runs/20260315_152418/results/batch-N.raw.txt` | Individual batch output targets |

---

## Quick Start (TL;DR)

```bash
# 1. Read the prompt
type .desloppify\external_review_sessions\ext_20260315_152913_6536bd28\claude_launch_prompt.md

# 2. Paste into Claude, get JSON output

# 3. Save JSON to:
#    .desloppify\external_review_sessions\ext_20260315_152913_6536bd28\review_result.json

# 4. Import and rescan
python -m desloppify review --external-submit --session-id ext_20260315_152913_6536bd28 --import .desloppify\external_review_sessions\ext_20260315_152913_6536bd28\review_result.json --scan-after-import

# 5. Check results
python -m desloppify status
```

---

**Questions?** The desloppify documentation is at: `python -m desloppify --help`
