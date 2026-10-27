# Check the actual coverprofile, not rounded per-package text from go test.
# Each block contributes its statement count to its package's denominator.
NR == 1 { next } # mode: atomic
{
    split($1, location, ":")
    package = location[1]
    sub(/\/[^/]+$/, "", package)
    statements[package] += $2
    if ($3 > 0) covered[package] += $2
    total += $2
    if ($3 > 0) totalCovered += $2
}
END {
    failed = 0
    for (package in statements) {
        percent = 100 * covered[package] / statements[package]
        printf "%-55s %5.1f%%\n", package, percent
        if (percent + 0.000001 < minimum) failed = 1
    }
    printf "%-55s %5.1f%%\n", "total", 100 * totalCovered / total
    if (100 * totalCovered / total + 0.000001 < minimum) failed = 1
    if (failed) print "coverage below " minimum "%" > "/dev/stderr"
    exit failed
}
