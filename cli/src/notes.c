#include <getopt.h>
#include <limits.h>
#include <stdio.h>

// system.h
enum {
  GETOPT_HELP_CHAR = (CHAR_MIN - 2),
  GETOPT_VERSION_CHAR = (CHAR_MIN - 3)
};

#define GETOPT_HELP_OPTION_DECL "help", no_argument, NULL, GETOPT_HELP_CHAR

#define GETOPT_VERSION_OPTION_DECL                                             \
  "version", no_argument, NULL, GETOPT_VERSION_CHAR

#define case_GETOPT_HELP_CHAR                                                  \
  case GETOPT_HELP_CHAR:                                                       \
    usage(EXIT_SUCCESS);                                                       \
    break;

static struct option const long_options[] = {
    {"backup", optional_argument, NULL, 'b'},
    {"directory", no_argument, NULL, 'F'},
    {"no-dereference", no_argument, NULL, 'n'},
    {"no-target-directory", no_argument, NULL, 'T'},
    {"force", no_argument, NULL, 'f'},
    {"interactive", no_argument, NULL, 'i'},
    {"suffix", required_argument, NULL, 'S'},
    {"target-directory", required_argument, NULL, 't'},
    {"logical", no_argument, NULL, 'L'},
    {"physical", no_argument, NULL, 'P'},
    {"relative", no_argument, NULL, 'r'},
    {"symbolic", no_argument, NULL, 's'},
    {"verbose", no_argument, NULL, 'v'},
    {GETOPT_HELP_OPTION_DECL},
    {GETOPT_VERSION_OPTION_DECL},
    {NULL, 0, NULL, 0}};
