---
name: squash-merge-worktree
description: Use when done working on a task in a worktree and ready to merge back to master
user-invocable: true
---

# Squash Merge Worktree

Squash merge the current worktree branch back to master and clean up.

## Critical: Worktree Context

**You are in a worktree.** You cannot `git checkout master` from here - it will fail. You must cd to the main repo directory first.

## Steps

### 1. Verify Clean State

```bash
git status
```

All changes must be committed before proceeding.

### 2. Get Branch Info

```bash
BRANCH=$(git branch --show-current)
WORKTREE_PATH=$(pwd)
MAIN_REPO=$(git worktree list | grep -v "/.worktrees/" | head -1 | awk '{print $1}')
```

### 3. Navigate to Main Repo

```bash
cd "$MAIN_REPO"
```

### 4. Squash Merge

```bash
git checkout master
git pull
git merge --squash "$BRANCH"
```

### 5. Commit with Meaningful Message

Create a single commit summarizing all the work. Follow the `[Category] Description` format from CLAUDE.md.

### 6. Clean Up Worktree

```bash
./scripts/task-cancel -f "$TASK_NAME"
```

The `-f` flag skips confirmation (non-interactive).

**Note:** The task name is the directory name under `~/.worktrees/shisho/` (e.g., if worktree is at `~/.worktrees/shisho/my-feature`, task name is `my-feature`).
