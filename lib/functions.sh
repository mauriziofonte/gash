#!/usr/bin/env bash

#########################################
#          Utility Functions            #
#########################################

# Print help message for a function if requested.
function needs_help() {
    local program="$1"
    local usage="$2"
    local help="$3"
    local user_input="$4"

    if [[ "$user_input" == "--help" || "$user_input" == "-h" ]]; then
        echo -e " 💡 \033[38;5;214m${program}\033[0m"
        echo -e "    \033[1;97mUsage:\033[0m \033[1;96m${usage}\033[0m"
        echo -e "    \033[1;97m${help}\033[0m"
        return 0
    fi

    return 1
}

function needs_confirm_prompt() {
    command printf '%b' "$1 \e[1;37m(y/N):\033[0m "
    read -r REPLY < /dev/tty
    if [[ "$REPLY" =~ ^[Yy]$ ]]; then
        return 0
    fi
    return 1
}

function print_error() {
  echo -e " ⛔ \033[1;31mError:\033[0m \e[1;37m$1\033[0m"
}

# List the top 100 largest files in a directory.
largest_files() {
    needs_help "largest_files" "largest_files [PATH]" "Lists the top 100 largest files in PATH (or current directory if not specified), sorted by size." "$1" && return

    local dir="${1:-.}"

    # fail if the directory does not exist
    if [ ! -d "$dir" ]; then
        print_error "Directory '$dir' does not exist."
        return 1
    fi

    find "$dir" -type f -printf '%s %p %TY-%Tm-%Td %TH:%TM\n' 2>/dev/null | \
    sort -nr -k1 | \
    awk '{ printf "\033[1;33m%-12s\033[0m \033[0;36m%-50s\033[0m \033[1;37m%s %s\033[0m\n", $1/1024/1024 "MB", $2, $3, $4 }' | \
    head -n 100

    return 0
}

# List the top 100 largest directories in a directory.
largest_dirs() {
    needs_help "largest_dirs" "largest_dirs [PATH]" "Lists the top 100 largest directories in PATH (or current directory if not specified), sorted by size." "$1" && return

    local dir="${1:-.}"
    
    # fail if the directory does not exist
    if [ ! -d "$dir" ]; then
        print_error "Directory '$dir' does not exist."
        return 1
    fi

    du -sm "$dir"/* 2>/dev/null | \
    sort -nr | \
    awk '{ printf "\033[1;33m%-12s\033[0m \033[0;36m%-50s\033[0m\n", $1 "MB", $2 }' | \
    head -n 100

    return 0
}

# Find directories exceeding a specified size and list their largest file modification time.
find_large_dirs() {
    # fail if we lack support for numfmt
    if ! command -v numfmt >/dev/null 2>&1; then
        print_error "This function requires the 'numfmt' utility, which is not available."
        return 1
    fi

    needs_help "find_large_dirs" "find_large_dirs [--size SIZE] [DIRECTORY]" "Finds directories larger than SIZE (default 20M) and lists their size and modification time of their largest file." "$1" && return

    local dir="."
    local size_threshold="20M"

    while [[ "$#" -gt 0 ]]; do
        case $1 in
            --size)
                size_threshold="$2"
                shift
                ;;
            *)
                dir="$1"
                ;;
        esac
        shift
    done

    # fail if the directory does not exist
    if [ ! -d "$dir" ]; then
        print_error "Directory '$dir' does not exist."
        return 1
    fi

    # Convert size threshold to kilobytes.
    local size_kb=$(numfmt --from=auto "${size_threshold}")

    find "$dir" -type d 2>/dev/null | while read -r d; do
        local total_size=$(du -sk "$d" 2>/dev/null | cut -f1)
        if [ "$total_size" -ge "$size_kb" ]; then
            local largest_file=$(find "$d" -type f -printf '%s\t%TY-%Tm-%Td %TH:%TM:%TS\t%p\n' 2>/dev/null | sort -nr | head -n1 | cut -f2)
            printf "\033[1;33m%-12s\033[0m \033[0;36m%-80s\033[0m \033[1;37m%-20s\033[0m\n" "$(du -sh "$d" 2>/dev/null | cut -f1)" "$d" "${largest_file:-N/A}"
        fi
    done | sort -rh

    return 0
}

# Display disk usage for specific filesystem types.
disk_usage_fs() {
    needs_help "disk_usage_fs" "disk_usage_fs" "Displays disk usage for specific filesystem types, formatted for easy reading." "$1" && return

    df -hT | awk '
    BEGIN {printf "%-20s %-8s %-8s %-8s %-8s %-6s %-20s\n", "Filesystem", "Type", "Size", "Used", "Avail", "Use%", "Mountpoint"}
    $2 ~ /(ext[2-4]|xfs|btrfs|zfs|f2fs|fat|vfat|ntfs)/ {
        printf "\033[1;33m%-20s\033[0m \033[0;36m%-8s\033[0m \033[1;37m%-8s\033[0m \033[1;37m%-8s\033[0m \033[1;37m%-8s\033[0m \033[38;5;214m%-6s\033[0m %-20s\n", $1, $2, $3, $4, $5, $6, $7
    }'

    return 0
}

# Search command history with colored output, removing duplicates, and avoiding self-call
hgrep() {
    needs_help "hgrep" "hgrep PATTERN" "Searches the bash history for commands matching PATTERN." "$1" && return

    # Extract the relevant parts (ignoring history line numbers), remove duplicates, and avoid self-call
    history | grep -i -- "$@" | grep -v "hgrep" | \
    awk '{ $1=""; seen[$0]++; if (seen[$0]==1) print $0 }' | \
    awk '{ 
        printf "\033[1;32m%-5s\033[0m \033[0;36m%-20s\033[0m \033[1;37m%s\033[0m\n", NR, $1" "$2, substr($0, index($0,$3)); 
    }'

    return 0
}

# Dump all revisions of a file in a GIT repo into multiple separate files
git_dump_revisions() {
    needs_help "git_dump_revisions" "git_dump_revisions FILENAME" "Dump all revisions of a file in a GIT repo into multiple separate files. Example: git_dump_revisions path/to/somefile.txt" "$1" && return

    # Check if we're in a git repository
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        print_error "Not in a git repository."
        return 1
    fi

    # Check if the file exists in the repository
    if [ ! -f "$1" ]; then
        print_error "File '$1' does not exist in the repository."
        return 1
    fi

    local file="$1"
    local index=1

    for commit in $(git log --pretty=format:%h "$file"); do
        local padindex=$(printf %03d "$index")
        local out="$file.$padindex.$commit"
        local log="$out.logmsg"

        echo -e " 💡 \033[1;37mSaving version $index to file $out for commit $commit\033[0m"

        # Save commit log message in a separate log file
        echo "*******************************************************" > "$log"
        git log -1 --pretty=format:"%s%nAuthored by %an at %ai%n%n%b%n" "$commit" >> "$log"
        echo "*******************************************************" >> "$log"

        # Save the actual file content for the commit
        git show "$commit:./$file" > "$out"

        index=$((index + 1))
    done
}

# Function to create and apply a patch from a feature branch to the main branch
git_apply_feature_patch() {
    needs_help "git_apply_feature_patch" "git_apply_feature_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH" "Create and apply a patch from a feature branch to the main branch. Example: git_apply_feature_patch main old-feat 123456" "$1" && return

    # Ensure we're in a Git repository
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        print_error "Not in a Git repository."
        return 1
    fi

    # Define input arguments with descriptive names
    local main_branch="$1"     # Example: "main"
    local feature_branch="$2"  # Example: "old-feat"
    local commit_hash="$3"     # Example: the commit hash from the main branch before feature branch was created

    # Check if input arguments are provided
    if [ -z "$main_branch" ] || [ -z "$feature_branch" ] || [ -z "$commit_hash" ]; then
        needs_help "git_apply_feature_patch" "git_apply_feature_patch MAIN_BRANCH FEATURE_BRANCH COMMIT_HASH" "Create and apply a patch from a feature branch to the main branch. Example: git_apply_feature_patch main old-feat 123456" "help"
        return 1
    fi

    # Check if the main branch exists
    if ! git show-ref --verify --quiet "refs/heads/$main_branch"; then
        print_error "Main branch '$main_branch' does not exist."
        return 1
    fi

    # Check if the feature branch exists
    if ! git show-ref --verify --quiet "refs/heads/$feature_branch"; then
        print_error "Feature branch '$feature_branch' does not exist."
        return 1
    fi

    # Check if the commit hash is valid
    if ! git cat-file -e "$commit_hash" 2>/dev/null; then
        print_error "Commit hash '$commit_hash' is not valid."
        return 1
    fi

    # Start patching process
    echo -e " 💡 \033[1;37m1. Checking out to main branch '$main_branch'\033[0m..."
    git checkout "$main_branch" && git pull origin "$main_branch"

    if [ $? -ne 0 ]; then
        print_error "Failed to checkout or pull the main branch."
        return 1
    fi

    echo -e " 💡 \033[1;37m2. Checking out to feature branch '$feature_branch'\033[0m..."
    git checkout "$feature_branch"

    if [ $? -ne 0 ]; then
        print_error "Failed to checkout the feature branch."
        return 1
    fi

    echo -e " 💡 \033[1;37m3. Generating patch from '$feature_branch' since commit '$commit_hash'\033[0m..."
    patch_file="${feature_branch}.patch"
    git diff-index "$commit_hash" --binary > "$patch_file"

    if [ $? -ne 0 ]; then
        print_error "Failed to create patch file."
        return 1
    fi

    echo -e " 💡 \033[1;37m4. Applying patch to '$main_branch'\033[0m..."
    git checkout "$main_branch" && git apply --3way "$patch_file"

    if [ $? -ne 0 ]; then
        print_error "Failed to apply the patch."
        return 1
    fi

    echo -e " ✅ \033[1;37mDone. Patch applied successfully from '$feature_branch' to '$main_branch'.\033[0m"
}

# List all tags (local or remote)
gtags() {
    needs_help "gtags" "gtags" "Lists all local and remote tags." "$1" && return

    # Check if we're in a git repository
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        print_error "Not in a git repository."
        return 1
    fi

    echo -e "\033[1;34mLocal tags:\033[0m"
    git tag -l
    echo -e "\033[1;34mRemote tags:\033[0m"
    git ls-remote --tags origin | awk '{print $2}' | sed 's|refs/tags/||'

    return 0
}

# Create an annotated tag and push it to the remote
gadd_tag() {
    needs_help "gadd_tag" "gadd_tag <tag_name> \"<tag_message>\"" "Creates an annotated tag and pushes it to remote." "$1" && return

    # Check if we're in a git repository
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        print_error "Not in a git repository."
        return 1
    fi

    if [[ -z "$1" ]]; then
        print_error "Please specify a tag name."
        return 1
    fi

    local tag_name="$1"
    local tag_message="${2:-"Release $tag_name"}"

    # Check if the tag already exists locally or remotely
    if git rev-parse "$tag_name" >/dev/null 2>&1 || git ls-remote --tags origin | grep -q "refs/tags/$tag_name"; then
        print_error "Tag '$tag_name' already exists."
        return 1
    fi

    # Create the tag
    git tag -a "$tag_name" -m "$tag_message"
    echo -e "\033[1;32mTag '$tag_name' created.\033[0m"

    # Push the tag to remote
    git push origin "$tag_name"
    echo -e "\033[1;32mTag '$tag_name' pushed to remote.\033[0m"

    return 0
}

# Delete a tag locally and on the remote
gdel_tag() {
    needs_help "gdel_tag" "gdel_tag <tag_name>" "Deletes a tag both locally and on remote." "$1" && return

    # Check if we're in a git repository
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        print_error "Not in a git repository."
        return 1
    fi

    if [[ -z "$1" ]]; then
        print_error "Please specify a tag name to delete."
        echo -e "\033[1;32mUsage: gdel_tag <tag_name>\033[0m"
        return 1
    fi

    local tag_name="$1"

    # Check if the tag exists locally or remotely
    if ! git rev-parse "$tag_name" >/dev/null 2>&1 && ! git ls-remote --tags origin | grep -q "refs/tags/$tag_name"; then
        print_error "Tag '$tag_name' does not exist."
        return 1
    fi

    # Delete the local tag if it exists
    if git rev-parse "$tag_name" >/dev/null 2>&1; then
        git tag -d "$tag_name" && echo -e "\033[1;32mLocal tag '$tag_name' deleted.\033[0m"
    fi

    # Delete the tag on the remote if it exists
    if git ls-remote --tags origin | grep -q "refs/tags/$tag_name"; then
        git push origin --delete "$tag_name" && echo -e "\033[1;32mRemote tag '$tag_name' deleted.\033[0m"
    fi

    return 0
}

# Stop all running Docker containers
docker_stop_all() {
    # do we have docker installed?
    if ! command -v docker >/dev/null 2>&1; then
        print_error "Docker is not installed."
        return 1
    fi

    echo -e " 💡 \033[1;37mStopping Docker Containers...\033[0m"
    docker stop $(docker ps -aq)
}

# Start all stopped Docker containers
docker_start_all() {
    # do we have docker installed?
    if ! command -v docker >/dev/null 2>&1; then
        print_error "Docker is not installed."
        return 1
    fi

    echo -e " 💡 \033[1;37mStarting Docker Containers...\033[0m"
    docker start $(docker ps -aq)
}

# Remove all Docker containers, images, volumes, and networks and clean up the environment
docker_prune_all() {
    # do we have docker installed?
    if ! command -v docker >/dev/null 2>&1; then
        print_error "Docker is not installed."
        return 1
    fi

    needs_confirm_prompt " ⚠️ \033[1;31mReally sure you want to remove all Docker containers, images, volumes, and networks?\033[0m"
    if [ $? -eq 1 ]; then
        return 0
    fi

    echo -e " 💡 \033[1;37m1. Stopping Docker Containers...\033[0m"
    docker stop $(docker ps -aq)

    echo -e " 💡 \033[1;37m2. Removing Docker Containers...\033[0m"
    docker rm $(docker ps -aq)

    echo -e " 💡 \033[1;37m3. Removing Docker Images...\033[0m"
    docker rmi $(docker images -q)

    echo -e " 💡 \033[1;37m4. Removing Docker Volumes...\033[0m"
    docker volume rm $(docker volume ls -q)

    echo -e " 💡 \033[1;37m5. Removing Docker Networks...\033[0m"
    docker network rm $(docker network ls -q | grep -v "bridge\|host\|none")

    echo -e " 💡 \033[1;37m6. Cleaning Docker Environment...\033[0m"
    docker system prune --volumes -a --force
}

# Prints all available colors with ANSI escape codes.
function all_colors() {
    for x in 0 1 4 5 7 8; do
        for i in seq 30 37; do
            for a in seq 40 47; do
                echo -ne "\e[$x;$i;$a""m\\\e[$x;$i;$a""m\e[0;37;40m "
            done
            echo
        done
    done
    echo ""
}

# Add 'please' command to re-run the previous command with sudo
please() {
    if [ "$EUID" -ne 0 ]; then
        if [ "$#" -eq 0 ]; then
            sudo $(fc -ln -1)
        else
            sudo "$@"
        fi
    else
        if [ "$#" -eq 0 ]; then
            $(fc -ln -1)
        else
            "$@"
        fi
    fi
}

# Create a function to make directory and cd into it
mkcd() {
    needs_help "mkcd" "mkcd DIRECTORY" "Creates a new directory and changes into it." "$1" && return

    mkdir -p "$1" && cd "$1"
}

# Function to extract various archive types (case-insensitive) with optional output directory.
extract() {
    needs_help "extract" "extract ARCHIVE_FILE [OUTPUT_DIR]" "Extracts the ARCHIVE_FILE in the current directory or the specified OUTPUT_DIR." "$1" && return

    local archive_file="$1"
    local output_dir="${2:-.}"

    if [ ! -f "$archive_file" ]; then
        print_error "'$archive_file' does not exist."
        return 1
    fi

    if [ ! -d "$output_dir" ]; then
        mkdir -p "$output_dir"
        if [ $? -ne 0 ]; then
            print_error "Failed to create output directory '$output_dir'."
            return 1
        fi
    fi

    shopt -s nocasematch
    case "$archive_file" in
        *.tar.bz2)   tar xvjf "$archive_file" -C "$output_dir"    ;;
        *.tar.gz)    tar xvzf "$archive_file" -C "$output_dir"    ;;
        *.bz2)       bunzip2 -c "$archive_file" > "$output_dir/$(basename "$archive_file" .bz2)"    ;;
        *.rar)       unrar x "$archive_file" "$output_dir"        ;;
        *.gz)        gunzip -c "$archive_file" > "$output_dir/$(basename "$archive_file" .gz)"      ;;
        *.tar)       tar xvf "$archive_file" -C "$output_dir"     ;;
        *.tbz2)      tar xvjf "$archive_file" -C "$output_dir"    ;;
        *.tgz)       tar xvzf "$archive_file" -C "$output_dir"    ;;
        *.zip)       unzip "$archive_file" -d "$output_dir"       ;;
        *.z)         uncompress "$archive_file" -c > "$output_dir/$(basename "$archive_file" .z)"   ;;
        *.7z)        7z x "$archive_file" -o"$output_dir"         ;;
        *)           echo "Error: Cannot extract '$archive_file', unsupported file type." ;;
    esac
    shopt -u nocasematch
}

# Create a backup of a file with a timestamp suffix
backup_file() {
    local file="$1"

    if [ ! -f "$file" ]; then
        print_error "File '$file' does not exist."
        return 1
    fi

    local backup="${file}_backup_$(date +%Y%m%d%H%M%S)"
    cp -v "$file" "$backup"

    if [ ! -f "$backup" ]; then
        print_error "Failed to create backup file."
        return 1
    fi
}

# List all empty directories in the specified path (default: current directory)
list_empty_dirs() {
    local dir="${1:-.}"

    if [ ! -d "$dir" ]; then
        print_error "Directory '$dir' does not exist."
        return 1
    fi

    find "$dir" -type d -empty
}

# Get your public IP address
myip() {
    # check for wget / curl and use one of them to fetch the IP from https://ipinfo.io/ip
    if command -v wget >/dev/null 2>&1; then
        IP=$(wget -qO- https://ipinfo.io/ip)
        echo -e " ✅ \033[1;37mPublic IP Address:\033[0m \033[0;36m$IP\033[0m"
    elif command -v curl >/dev/null 2>&1; then
        IP=$(curl -s https://ipinfo.io/ip)
        echo -e " ✅ \033[1;37mPublic IP Address:\033[0m \033[0;36m$IP\033[0m"
    else
        print_error "This function requires either 'wget' or 'curl' to be installed."
        return 1
    fi
}

# Search for a process by name
psgrep() {
    local process_name="$1"
    local result=$(ps aux | grep -i "$process_name" | grep -v grep)

    if [ -n "$result" ]; then
        echo -e " 💡 Processes matching \033[1;37m$process_name\033[0m:"
        echo "$result" | awk '{ printf "   \033[1;33m%-8s\033[0m \033[0;36m%-12s\033[0m %-4s \033[1;37m%-40s\033[0m\n", $2, $1, $3, $11 }'
    else
        print_error "No process found with name '$process_name'."
    fi
}

# Kill all processes by name
pskill() {
    local pids=$(ps aux | grep -i "$1" | grep -v grep | awk '{print $2}')
    
    if [ -n "$pids" ]; then
        for pid in $pids; do
            kill -9 "$pid"
            echo -e " 💡 \033[1;37mProcess with PID $pid killed.\033[0m"
        done
    else
        print_error "No process found with name '$1'."
    fi
}

# Kill all processes by port
portkill() {
    local pids=$(lsof -t -i:"$port")
    
    if [ -n "$pids" ]; then
        for pid in $pids; do
            kill -9 "$pid"
            echo -e " 💡 \033[1;37mProcess on Port $port with PID $pid killed.\033[0m\n"
        done
    else
        print_error "No process found on port $port."
    fi
}

# Stop well-known services like Apache, Nginx, MySQL, MariaDB, Pgsql, redis, memcached, etc.
stop_services() {
    local services=("apache2" "nginx" "mysql" "mariadb" "postgresql" "mongodb" "redis" "memcached" "docker")
    local force_flag="$1"
    
    if [[ "$force_flag" != "--force" ]]; then
        needs_confirm_prompt " ⚠️ \033[1;31mReally sure you want to stop all well-known services?\033[0m"
        if [ $? -eq 1 ]; then
            return 0
        fi
    fi

    for service in "${services[@]}"; do
        if systemctl is-active --quiet "$service"; then
            echo -e " 💡 \033[1;37mStopping $service service...\033[0m"
            sudo systemctl stop "$service"
        fi
    done
}

# Function to uninstall Gash and clean up configurations
gash_uninstall() {
    # If we don't have a ~/.gash directory, simply exit
    if [ ! -d "$HOME/.gash" ]; then
        echo -e " ⛔ \033[1;31mGash is not installed on this system. Maybe you've just uninstalled it?\033[0m"
        return 1
    fi

    echo -e " ⚠️ \033[1;31mThis will completely remove Gash and its configurations from your system.\033[0m"
    needs_confirm_prompt " ⚠️ \033[1;31mDo you want to continue?\033[0m"
    if [ $? -eq 1 ]; then
        echo -e " ⛔ \033[1;33m Uninstallation aborted.\033[0m"
        return 0
    fi

    # Define profile files to check
    profile_files=( "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.profile" )

    # Remove Gash sourcing lines from profiles
    for profile_file in "${profile_files[@]}"; do
        if [ -f "$profile_file" ]; then
            if command grep -qc 'source.*[./]gashrc' "$profile_file"; then
                echo -e " 💡 \033[1;32m Removing Gash block from: $profile_file\033[0m"
                # Remove the block between '# Load Gash Bash' and 'fi'
                sed -i.bak '/# Load Gash Bash/,/^fi$/d' "$profile_file"
            else
                echo -e " 💡 \033[1;33m No Gash block found in: $profile_file, skipping...\033[0m"
            fi
        fi
    done

    # Remove the ~/.gashrc file if it exists
    if [ -f "$HOME/.gashrc" ]; then
        echo -e " 💡 \033[1;32m Removing ~/.gashrc file...\033[0m"
        rm -f "$HOME/.gashrc" >/dev/null 2>&1

        # Failsafe: if the file still exists, try with sudo
        if [ -f "$HOME/.gashrc" ]; then
            echo -e " ⚠️ \033[1;33m Failed to remove ~/.gashrc file, trying with sudo...\033[0m"
            sudo rm -f "$HOME/.gashrc" >/dev/null 2>&1

            if [ -f "$HOME/.gashrc" ]; then
                echo -e " ⛔ \033[1;31m Failed to remove ~/.gashrc file, please remove it manually.\033[0m"
            fi
        fi
    else
        echo -e " 💡 \033[1;33m ~/.gashrc file not found, skipping...\033[0m"
    fi

    # Remove the Gash directory
    local GASH_DIR
    GASH_DIR="${GASH_DIR:-$HOME/.gash}"

    if [ -d "$GASH_DIR" ]; then
        echo -e " 💡 \033[1;32m Removing Gash at ~/.gash ...\033[0m"
        rm -rf "$GASH_DIR" > /dev/null 2>&1

        # Failsafe: if the directory still exists, try with sudo
        if [ -d "$GASH_DIR" ]; then
            echo -e " ⚠️ \033[1;33m Failed to remove Gash ~/.gash directory, trying with sudo...\033[0m"
            sudo rm -rf "$GASH_DIR" >/dev/null 2>&1

            if [ -d "$GASH_DIR" ]; then
                echo -e " ⛔ \033[1;31m Failed to remove Gash ~/.gash directory, please remove it manually.\033[0m"
            fi
        fi
    else
        echo -e " 💡 \033[1;33m Gash directory at ~/.gash not found, skipping...\033[0m"
    fi

    echo -e " ✅ \033[1;32mGash successfully uninstalled.\033[0m"
    echo -e " 💡 \033[1;37mPlease restart your terminal to apply changes.\033[0m"

    # Clean up backup files created by sed (if any)
    for profile_file in "${profile_files[@]}"; do
        if [ -f "${profile_file}.bak" ]; then
            rm -f "${profile_file}.bak"
        fi
    done
}

gash_inspiring_quote() {
    # Check if tput is available, and fallback to 60 if not
    if command -v tput &> /dev/null; then
        TTY_WIDTH=$(tput cols)
    else
        TTY_WIDTH=60
    fi

    local GASH_DIR
    GASH_DIR="${GASH_DIR:-$HOME/.gash}"
    local QUOTES_FILE="$GASH_DIR/quotes/list.txt"

    # Check if the quote file exists and is readable
    if [[ ! -r "$QUOTES_FILE" ]]; then
        return 1
    fi

    # Read all quotes into an array without changing IFS
    mapfile -t quotes < "$QUOTES_FILE"

    if [[ ${#quotes[@]} -eq 0 ]]; then
        return 1
    fi

    # Generate a random index
    local IDX=$(( RANDOM % ${#quotes[@]} ))

    # Output the random quote
    echo -e "\033[1;97m🌟\033[0m They said: \033[1;96m${quotes[$IDX]}\033[0m" | fold -s -w $TTY_WIDTH

    # unset the quotes array
    unset TTY_WIDTH QUOTES_FILE quotes
}

# Define custom help function
function gash_help() {
    # Display the built-in Bash help
    builtin help "$@"

    # If no specific help topic is requested, show Gash-specific help
    if [[ -z "$1" ]]; then
        echo
        echo -e "\e[1;37m===\033[0m \033[0;36mG\033[0;33ma\033[38;5;214ms\033[0;32mh \033[1;37mGash, Another SHell!\033[0m - \e[1;37mCustom Commands ===\033[0m"
        
        # List Gash-defined functions and their descriptions
        echo -e " > \e[0;33mlargest_files\033[0m \e[0;36m[PATH]\033[0m - \e[1;37mLists the top 100 largest files in PATH (or current directory if not specified), sorted by size.\033[0m"
        echo -e " > \e[0;33mlargest_dirs\033[0m \e[0;36m[PATH]\033[0m - \e[1;37mLists the top 100 largest directories in PATH (or current directory if not specified), sorted by size.\033[0m"
        echo -e " > \e[0;33mfind_large_dirs\033[0m \e[0;36m[--size SIZE] [DIRECTORY]\033[0m - \e[1;37mFinds directories larger than SIZE (default 20M) and lists their size and modification time of their largest file.\033[0m"
        echo -e " > \e[0;33mdisk_usage_fs\033[0m - \e[1;37mDisplays disk usage for specific filesystem types, formatted for easy reading.\033[0m"
        echo -e " > \e[0;33mhgrep\033[0m \e[0;36mPATTERN\033[0m - \e[1;37mSearches the bash history for commands matching PATTERN.\033[0m"
        echo -e " > \e[0;33mgit_dump_revisions\033[0m \e[0;36mFILENAME\033[0m - \e[1;37mDump all revisions of a file in a GIT repo into multiple separate files.\033[0m"
        echo -e " > \e[0;33mgit_apply_feature_patch\033[0m \e[0;36mMAIN_BRANCH FEATURE_BRANCH COMMIT_HASH\033[0m - \e[1;37mCreate and apply a patch from a feature branch to the main branch.\033[0m"
        echo -e " > \e[0;33mgtags\033[0m - \e[1;37mLists all local and remote tags.\033[0m"
        echo -e " > \e[0;33mgadd_tag\033[0m \e[0;36m<tag_name> \"<tag_message>\"\033[0m - \e[1;37mCreates an annotated tag and pushes it to remote.\033[0m"
        echo -e " > \e[0;33mgdel_tag\033[0m \e[0;36m<tag_name>\033[0m - \e[1;37mDeletes a tag both locally and on remote.\033[0m"
        echo -e " > \e[0;33mdocker_prune_all\033[0m - \e[1;37mRemove all Docker containers, images, volumes, and networks.\033[0m"
        echo -e " > \e[0;33mplease\033[0m - \e[1;37mRe-runs the previous command with sudo.\033[0m"
        echo -e " > \e[0;33mmkcd\033[0m \e[0;36mDIRECTORY\033[0m - \e[1;37mCreates a new directory and changes into it.\033[0m"
        echo -e " > \e[0;33mextract\033[0m \e[0;36mARCHIVE_FILE [OUTPUT_DIR]\033[0m - \e[1;37mExtracts the ARCHIVE_FILE in the current directory or the specified OUTPUT_DIR.\033[0m"
        echo -e " > \e[0;33mbackup_file\033[0m \e[0;36mFILE\033[0m - \e[1;37mCreates a backup of a file with a timestamp suffix.\033[0m"
        echo -e " > \e[0;33mlist_empty_dirs\033[0m \e[0;36m[DIRECTORY]\033[0m - \e[1;37mList all empty directories in the specified path (default: current directory).\033[0m"
        echo -e " > \e[0;33mmyip\033[0m - \e[1;37mGet your public IP address.\033[0m"
        echo -e " > \e[0;33mpsgrep\033[0m \e[0;36mPROCESS_NAME\033[0m - \e[1;37mSearch for a process by name.\033[0m"
        echo -e " > \e[0;33mpskill\033[0m \e[0;36mPROCESS_NAME\033[0m - \e[1;37mKill all processes by name.\033[0m"
        echo -e " > \e[0;33mportkill\033[0m \e[0;36mPORT\033[0m - \e[1;37mKill all processes by port.\033[0m"
        echo -e " > \e[0;33mstop_services\033[0m \e[0;36m[--force]\033[0m - \e[1;37mStop well-known services like Apache, Nginx, MySQL, MariaDB, Pgsql, redis, memcached, etc.\033[0m"
        echo -e " > \e[0;33mgash_inspiring_quote\033[0m - \e[1;37mDisplay an inspiring quote, to enlighten your day.\033[0m"
        echo -e " > \e[0;33mgash_uninstall\033[0m - \e[1;37mUninstall Gash and clean up configurations.\033[0m"

        echo -e " > \e[0;33m..\033[0m - \e[1;37mChange to the parent directory.\033[0m"
        echo -e " > \e[0;33m...\033[0m - \e[1;37mChange to the parent's parent directory.\033[0m"
        echo -e " > \e[0;33mcd..\033[0m - \e[1;37mChange to the parent directory.\033[0m"
        echo -e " > \e[0;33m...\033[0m - \e[1;37mChange to the parent's parent directory.\033[0m"
        echo -e " > \e[0;33m....\033[0m - \e[1;37mChange to the parent's parent's parent directory.\033[0m"
        echo -e " > \e[0;33m.....\033[0m - \e[1;37mChange to the parent's parent's parent directory.\033[0m"
        echo -e " > \e[0;33m.4\033[0m - \e[1;37mChange to the parent's parent's parent directory.\033[0m"
        echo -e " > \e[0;33m.5\033[0m - \e[1;37mChange to the parent's parent's parent's parent directory.\033[0m"
        echo -e " > \e[0;33mports\033[0m - \e[1;37mDisplay listening ports.\033[0m"
        if grep -qi "microsoft" /proc/version && [ -n "$WSLENV" ]; then
            echo -e " > \e[0;33mwslrestart\033[0m - \e[1;37mRestart WSL.\033[0m"
            echo -e " > \e[0;33mwslshutdown\033[0m - \e[1;37mShutdown WSL.\033[0m"
            echo -e " > \e[0;33mexplorer\033[0m - \e[1;37mOpen the current directory in Windows Explorer.\033[0m"
            echo -e " > \e[0;33mtaskmanager\033[0m - \e[1;37mOpen Task Manager.\033[0m"
        fi
        if command -v git >/dev/null 2>&1; then
            echo -e " > \e[0;33mgl or glog\033[0m - \e[1;37mDisplay a pretty git log.\033[0m"
            echo -e " > \e[0;33mgst or gstatus\033[0m - \e[1;37mDisplay the git status.\033[0m"
            echo -e " > \e[0;33mga or gadd\033[0m - \e[1;37mAdd files to the git index.\033[0m"
            echo -e " > \e[0;33mgc or gcommit\033[0m - \e[1;37mCommit changes to the git repository.\033[0m"
            echo -e " > \e[0;33mgp or gpush\033[0m - \e[1;37mPush changes to the git repository.\033[0m"
            echo -e " > \e[0;33mgco or gcheckout\033[0m - \e[1;37mSwitch branches or restore working tree files.\033[0m"
            echo -e " > \e[0;33mgb or gbranch\033[0m - \e[1;37mList, create, or delete branches.\033[0m"
            echo -e " > \e[0;33mgd or gdiff\033[0m - \e[1;37mShow changes between commits, commit and working tree, etc.\033[0m"
            echo -e " > \e[0;33mgt or gtags\033[0m - \e[1;37mCreate, list, delete or verify a tag object signed with GPG.\033[0m"
            echo -e " > \e[0;33mgta or gadd_tag\033[0m - \e[1;37mCreate an annotated tag and push it to the remote.\033[0m"
            echo -e " > \e[0;33mgtd or gdel_tag\033[0m - \e[1;37mDelete a tag both locally and on remote.\033[0m"
        fi
        if command -v docker >/dev/null 2>&1; then
            echo -e " > \e[0;33mdcls\033[0m - \e[1;37mList all Docker containers.\033[0m"
            echo -e " > \e[0;33mdclsr\033[0m - \e[1;37mList running Docker containers.\033[0m"
            echo -e " > \e[0;33mdils\033[0m - \e[1;37mList all Docker images.\033[0m"
            echo -e " > \e[0;33mdcrm\033[0m - \e[1;37mRemove all Docker containers.\033[0m"
            echo -e " > \e[0;33mdirm\033[0m - \e[1;37mRemove all Docker images.\033[0m"
            echo -e " > \e[0;33mdstop\033[0m - \e[1;37mStop a Docker container.\033[0m"
            echo -e " > \e[0;33mdstopall\033[0m - \e[1;37mStop all Docker containers.\033[0m"
            echo -e " > \e[0;33mdocker_stop_all\033[0m - \e[1;37mStop all Docker containers.\033[0m"
            echo -e " > \e[0;33mdstart\033[0m - \e[1;37mStart a Docker container.\033[0m"
            echo -e " > \e[0;33mdstartall\033[0m - \e[1;37mStart all Docker containers.\033[0m"
            echo -e " > \e[0;33mdocker_start_all\033[0m - \e[1;37mStart all Docker containers.\033[0m"
            echo -e " > \e[0;33mdexec\033[0m - \e[1;37mExecute a command in a running Docker container.\033[0m"
            echo -e " > \e[0;33mdrm\033[0m - \e[1;37mRemove a Docker container.\033[0m"
            echo -e " > \e[0;33mdrmi\033[0m - \e[1;37mRemove a Docker image.\033[0m"
            echo -e " > \e[0;33mdlogs\033[0m - \e[1;37mShow logs of a Docker container.\033[0m"
            echo -e " > \e[0;33mdinspect\033[0m - \e[1;37mInspect a Docker object.\033[0m"
            echo -e " > \e[0;33mdnetls\033[0m - \e[1;37mList all Docker networks.\033[0m"
            echo -e " > \e[0;33mdpruneall\033[0m - \e[1;37mRemove all Docker containers, images, volumes, and networks.\033[0m"
            echo -e " > \e[0;33mdocker_prune_all\033[0m - \e[1;37mRemove all Docker containers, images, volumes, and networks.\033[0m"
        fi
    fi
}