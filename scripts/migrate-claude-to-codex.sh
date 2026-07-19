#!/usr/bin/env bash
# Copy project-local Claude skills into Codex's repository skill location and
# translate Claude path-scoped rules into lazy-loaded Codex skills.
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
source_dir="$repo_root/.claude"
skills_dir="$repo_root/.agents/skills"

if [[ ! -d "$source_dir/skills" || ! -d "$source_dir/rules" ]]; then
    echo "Expected .claude/skills and .claude/rules under $repo_root" >&2
    exit 1
fi

mkdir -p "$skills_dir"

# Claude skills already use the Agent Skills SKILL.md format. Copy their
# complete directories so any future bundled resources come across unchanged.
for source_skill in "$source_dir"/skills/*; do
    [[ -d "$source_skill" && -f "$source_skill/SKILL.md" ]] || continue
    skill_name=$(basename "$source_skill")
    mkdir -p "$skills_dir/$skill_name"
    cp -R "$source_skill/." "$skills_dir/$skill_name/"
done

write_rule_skill() {
    local name=$1 description=$2 rule=$3
    local destination="$skills_dir/$name"
    mkdir -p "$destination"
    {
        printf '%s\n' '---'
        printf 'name: %s\n' "$name"
        printf 'description: %s\n' "$description"
        printf '%s\n\n' '---'
        # Strip Claude frontmatter, retaining the rule body unchanged.
        awk '
            NR == 1 && $0 == "---" { frontmatter = 1; next }
            frontmatter && $0 == "---" { frontmatter = 0; next }
            !frontmatter { print }
        ' "$rule"
    } >"$destination/SKILL.md"
}

write_rule_skill \
    go-development \
    'Apply this project’s Go coding practices when creating, modifying, reviewing, or debugging Go source files. Covers design, errors, dependencies, and required verification; do not use for non-Go work.' \
    "$source_dir/rules/go-general.md"
write_rule_skill \
    go-naming \
    'Apply this project’s Go naming conventions when creating, renaming, or reviewing Go packages, types, functions, methods, interfaces, variables, errors, or files. Do not use for non-Go changes.' \
    "$source_dir/rules/go-naming.md"
write_rule_skill \
    go-testing \
    'Apply this project’s Go testing conventions when adding, modifying, debugging, or reviewing Go tests, including *_test.go files. Do not use for non-Go tests.' \
    "$source_dir/rules/go-testing.md"

echo "Migrated Claude skills and rules to $skills_dir"
