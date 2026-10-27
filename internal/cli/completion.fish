# fish completion for casemv. It asks `casemv __complete` directly, so the
# directory and file-extension filters that cobra's fish script drops apply.

function __casemv_complete
    set -l tokens (commandline -opc)
    # Unescaped, so "My\ D" matches "My Docs/"; an unfinished quote stays raw.
    set -l current (commandline -ct | string unescape 2>/dev/null)
    or set current (commandline -ct)

    set -l response ($tokens[1] __complete $tokens[2..-1] "$current" 2>/dev/null)
    or return
    set -l directive (string replace -r -- '^:' '' $response[-1])
    string match -qr -- '^[0-9]+$' $directive
    or return
    set -e response[-1]

    # Directive bits, as cobra defines them.
    set -l error 1
    set -l no_file_completion 4
    set -l filter_file_extensions 8
    set -l filter_directories 16

    if test (math "bitand($directive, $error)") -ne 0
        return
    end
    # fish's path helpers take the token as typed, quotes included.
    if test (math "bitand($directive, $filter_directories)") -ne 0
        __fish_complete_directories
        return
    end
    if test (math "bitand($directive, $filter_file_extensions)") -ne 0
        __casemv_complete_extensions $response
        return
    end

    printf '%s\n' $response
    if test (count $response) -eq 0; and test (math "bitand($directive, $no_file_completion)") -eq 0
        __fish_complete_path (commandline -ct)
    end
end

# Files with one of the given extensions, plus directories to reach them.
# Not __fish_complete_suffix: it offers every file, matching ones first.
function __casemv_complete_extensions
    set -l extensions (string join '|' (string escape --style=regex -- $argv))
    for candidate in (__fish_complete_path (commandline -ct))
        set -l path (string split -f1 -- \t $candidate)
        if string match -q -- '*/' $path; or string match -qr -- "\.($extensions)\$" $path
            echo $path
        end
    end
end

complete -c casemv -e
complete -c casemv -f -a '(__casemv_complete)'
