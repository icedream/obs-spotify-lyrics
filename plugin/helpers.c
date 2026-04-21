#include <obs-module.h>

void blog_string(int log_level, const char *string)
{
	blog(log_level, "[spotify-lyrics] %s", string);
}

void call_enum_proc(obs_source_enum_proc_t proc, obs_source_t *parent, obs_source_t *child, void *param)
{
	if (proc) proc(parent, child, param);
}
