#include <obs-module.h>

void blog_string(int log_level, const char *string)
{
	blog(log_level, "[spotify-lyrics] %s", string);
}
