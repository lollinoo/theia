#!/usr/bin/env bash
set -euo pipefail

readonly ALLOWED_TYPES='build|ci|chore|docs|feat|fix|perf|refactor|revert|style|test'
readonly HEADER_PATTERN="^(${ALLOWED_TYPES})(\\([[:alnum:]./_-]+\\))?(!)?: [^[:space:]].*$"
readonly ZERO_SHA='0000000000000000000000000000000000000000'

print_usage() {
  cat <<'EOF'
Usage:
  scripts/validate-commit-msg.sh --file <commit-message-file>
  scripts/validate-commit-msg.sh --range <git-revision-range-or-sha>

Examples:
  scripts/validate-commit-msg.sh --file .git/COMMIT_EDITMSG
  scripts/validate-commit-msg.sh --range HEAD~3..HEAD
EOF
}

fail_validation() {
  local context=$1
  local message=$2

  printf 'ERROR [%s]: %s\n' "$context" "$message" >&2
  exit 1
}

trim_message_lines() {
  local first=0
  local last=$((${#MESSAGE_LINES[@]} - 1))
  local idx

  TRIMMED_LINES=()

  if [ "${#MESSAGE_LINES[@]}" -eq 0 ]; then
    return
  fi

  while [ "$first" -le "$last" ] && [ -z "${MESSAGE_LINES[$first]//[[:space:]]/}" ]; do
    first=$((first + 1))
  done

  while [ "$last" -ge "$first" ] && [ -z "${MESSAGE_LINES[$last]//[[:space:]]/}" ]; do
    last=$((last - 1))
  done

  for ((idx=first; idx<=last; idx++)); do
    TRIMMED_LINES+=("${MESSAGE_LINES[$idx]}")
  done
}

load_message_from_file() {
  local file_path=$1
  local comment_char
  local line

  if [ ! -f "$file_path" ]; then
    fail_validation "$file_path" "Commit message file does not exist."
  fi

  MESSAGE_LINES=()
  comment_char=$(git config --get core.commentChar 2>/dev/null || true)

  if [ -z "$comment_char" ] || [ "$comment_char" = "auto" ]; then
    comment_char="#"
  fi

  while IFS= read -r line || [ -n "$line" ]; do
    if [ -n "$comment_char" ] && [ "${line#"$comment_char"}" != "$line" ]; then
      continue
    fi

    MESSAGE_LINES+=("$line")
  done < "$file_path"

  trim_message_lines
}

load_message_from_text() {
  local raw_message=$1
  local line

  MESSAGE_LINES=()

  while IFS= read -r line || [ -n "$line" ]; do
    MESSAGE_LINES+=("$line")
  done < <(printf '%s' "$raw_message")

  trim_message_lines
}

validate_loaded_message() {
  local context=$1
  local subject
  local idx
  local has_body_content=0

  if [ "${#TRIMMED_LINES[@]}" -eq 0 ]; then
    fail_validation "$context" "Commit message is empty."
  fi

  subject=${TRIMMED_LINES[0]}

  case "$subject" in
    fixup!\ *|squash!\ *)
      fail_validation "$context" "Autosquash commit messages are not allowed. Rewrite the commit message to the conventional format."
      ;;
    Merge\ *)
      fail_validation "$context" "Merge commit messages must be rewritten to the conventional commit format."
      ;;
    Revert\ \"*\")
      fail_validation "$context" "Git's default revert message is not allowed. Use the conventional 'revert:' type instead."
      ;;
  esac

  if ! [[ "$subject" =~ $HEADER_PATTERN ]]; then
    fail_validation "$context" "Invalid conventional commit header: '$subject'. Expected '<type>(<scope>)?: <description>'."
  fi

  if [ "${#TRIMMED_LINES[@]}" -eq 1 ]; then
    return
  fi

  if [ -n "${TRIMMED_LINES[1]//[[:space:]]/}" ]; then
    fail_validation "$context" "Commit body or footer must be separated from the subject with a blank line."
  fi

  for ((idx=2; idx<${#TRIMMED_LINES[@]}; idx++)); do
    if [ -n "${TRIMMED_LINES[$idx]//[[:space:]]/}" ]; then
      has_body_content=1
      break
    fi
  done

  if [ "$has_body_content" -eq 0 ]; then
    fail_validation "$context" "Commit message contains a separator blank line but no body or footer content."
  fi
}

validate_commit_file() {
  local file_path=$1

  load_message_from_file "$file_path"
  validate_loaded_message "$file_path"
}

commit_range_to_rev_list_arg() {
  local revision=$1

  if [[ "$revision" == *..* ]]; then
    printf '%s' "$revision"
    return
  fi

  if [ "$revision" = "$ZERO_SHA" ]; then
    fail_validation "$revision" "Zero SHA is not a valid commit selector."
  fi

  printf '%s^!' "$revision"
}

validate_commit_range() {
  local revision=$1
  local rev_list_arg
  local commit_sha
  local commit_message
  local commit_count=0

  rev_list_arg=$(commit_range_to_rev_list_arg "$revision")

  while IFS= read -r commit_sha; do
    if [ -z "$commit_sha" ]; then
      continue
    fi

    commit_count=$((commit_count + 1))
    commit_message=$(git log -1 --format=%B "$commit_sha")
    load_message_from_text "$commit_message"
    validate_loaded_message "$commit_sha"
  done < <(git rev-list --reverse "$rev_list_arg")

  if [ "$commit_count" -eq 0 ]; then
    fail_validation "$revision" "No commits found to validate."
  fi
}

main() {
  if [ "$#" -eq 1 ] && { [ "$1" = "-h" ] || [ "$1" = "--help" ]; }; then
    print_usage
    exit 0
  fi

  if [ "$#" -ne 2 ]; then
    print_usage >&2
    exit 1
  fi

  case "$1" in
    --file)
      validate_commit_file "$2"
      ;;
    --range)
      validate_commit_range "$2"
      ;;
    *)
      print_usage >&2
      exit 1
      ;;
  esac
}

main "$@"
