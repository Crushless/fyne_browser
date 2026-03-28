//go:build cgo && (linux || darwin)

#include "cef_dynamic.h"

#include <dlfcn.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>

#if defined(OS_LINUX) || defined(__linux__)
#include "include/internal/cef_types_linux.h"
#endif

typedef struct {
  void* handle;
  int loaded;
  char error[512];

  const char* (*api_hash)(int version, int entry);
  int (*execute_process)(const cef_main_args_t* args,
                         cef_app_t* application,
                         void* windows_sandbox_info);
  int (*initialize)(const cef_main_args_t* args,
                    const struct _cef_settings_t* settings,
                    cef_app_t* application,
                    void* windows_sandbox_info);
  void (*shutdown)(void);
  void (*do_message_loop_work)(void);
  cef_browser_t* (*browser_host_create_browser_sync)(
      const cef_window_info_t* windowInfo,
      struct _cef_client_t* client,
      const cef_string_t* url,
      const struct _cef_browser_settings_t* settings,
      struct _cef_dictionary_value_t* extra_info,
      struct _cef_request_context_t* request_context);
  int (*string_utf8_to_utf16)(const char* src,
                              size_t src_len,
                              cef_string_utf16_t* output);
  int (*string_utf16_to_utf8)(const char16_t* src,
                              size_t src_len,
                              cef_string_utf8_t* output);
  void (*string_utf16_clear)(cef_string_utf16_t* str);
  void (*string_utf8_clear)(cef_string_utf8_t* str);
  void (*string_userfree_utf16_free)(cef_string_userfree_utf16_t str);
#if defined(OS_LINUX) || defined(__linux__)
  XDisplay* (*get_xdisplay)(void);
#endif
} fynecef_cef_library_t;

static fynecef_cef_library_t fynecef_cef = {0};

static void fynecef_set_cef_error(const char* format, ...) {
  va_list args;
  va_start(args, format);
  vsnprintf(fynecef_cef.error, sizeof(fynecef_cef.error), format, args);
  va_end(args);
}

static int fynecef_resolve_cef_symbol(void** target, const char* name) {
  const char* err = NULL;

  dlerror();
  *target = dlsym(fynecef_cef.handle, name);
  err = dlerror();
  if (err != NULL || *target == NULL) {
    fynecef_set_cef_error("resolve %s: %s", name,
                          err != NULL ? err : "symbol missing");
    return 0;
  }
  return 1;
}

int fynecef_cef_load_library(const char* library_path) {
  if (fynecef_cef.loaded) {
    return 1;
  }
  if (library_path == NULL || library_path[0] == '\0') {
    fynecef_set_cef_error("empty CEF library path");
    return 0;
  }

  dlerror();
  fynecef_cef.handle = dlopen(library_path, RTLD_NOW | RTLD_GLOBAL);
  if (fynecef_cef.handle == NULL) {
    const char* err = dlerror();
    fynecef_set_cef_error("dlopen %s: %s", library_path,
                          err != NULL ? err : "unknown error");
    return 0;
  }

  if (!fynecef_resolve_cef_symbol((void**)&fynecef_cef.api_hash,
                                  "cef_api_hash") ||
      !fynecef_resolve_cef_symbol((void**)&fynecef_cef.execute_process,
                                  "cef_execute_process") ||
      !fynecef_resolve_cef_symbol((void**)&fynecef_cef.initialize,
                                  "cef_initialize") ||
      !fynecef_resolve_cef_symbol((void**)&fynecef_cef.shutdown,
                                  "cef_shutdown") ||
      !fynecef_resolve_cef_symbol((void**)&fynecef_cef.do_message_loop_work,
                                  "cef_do_message_loop_work") ||
      !fynecef_resolve_cef_symbol(
          (void**)&fynecef_cef.browser_host_create_browser_sync,
          "cef_browser_host_create_browser_sync") ||
      !fynecef_resolve_cef_symbol((void**)&fynecef_cef.string_utf8_to_utf16,
                                  "cef_string_utf8_to_utf16") ||
      !fynecef_resolve_cef_symbol((void**)&fynecef_cef.string_utf16_to_utf8,
                                  "cef_string_utf16_to_utf8") ||
      !fynecef_resolve_cef_symbol((void**)&fynecef_cef.string_utf16_clear,
                                  "cef_string_utf16_clear") ||
      !fynecef_resolve_cef_symbol((void**)&fynecef_cef.string_utf8_clear,
                                  "cef_string_utf8_clear") ||
      !fynecef_resolve_cef_symbol(
          (void**)&fynecef_cef.string_userfree_utf16_free,
          "cef_string_userfree_utf16_free")
#if defined(OS_LINUX) || defined(__linux__)
      || !fynecef_resolve_cef_symbol((void**)&fynecef_cef.get_xdisplay,
                                     "cef_get_xdisplay")
#endif
  ) {
    char last_error[sizeof(fynecef_cef.error)];
    snprintf(last_error, sizeof(last_error), "%s", fynecef_cef.error);
    dlclose(fynecef_cef.handle);
    memset(&fynecef_cef, 0, sizeof(fynecef_cef));
    snprintf(fynecef_cef.error, sizeof(fynecef_cef.error), "%s", last_error);
    return 0;
  }

  fynecef_cef.loaded = 1;
  fynecef_cef.error[0] = '\0';
  return 1;
}

const char* fynecef_cef_last_error(void) {
  if (fynecef_cef.error[0] != '\0') {
    return fynecef_cef.error;
  }
  return "unknown CEF loader error";
}

const char* fynecef_cef_api_hash(int version, int entry) {
  if (!fynecef_cef.loaded) {
    fynecef_set_cef_error("CEF library is not loaded");
    return NULL;
  }
  return fynecef_cef.api_hash(version, entry);
}

int fynecef_cef_execute_process(const cef_main_args_t* args,
                                cef_app_t* application,
                                void* windows_sandbox_info) {
  if (!fynecef_cef.loaded) {
    fynecef_set_cef_error("CEF library is not loaded");
    return -1;
  }
  return fynecef_cef.execute_process(args, application, windows_sandbox_info);
}

int fynecef_cef_initialize(const cef_main_args_t* args,
                           const struct _cef_settings_t* settings,
                           cef_app_t* application,
                           void* windows_sandbox_info) {
  if (!fynecef_cef.loaded) {
    fynecef_set_cef_error("CEF library is not loaded");
    return 0;
  }
  return fynecef_cef.initialize(args, settings, application,
                                windows_sandbox_info);
}

void fynecef_cef_shutdown(void) {
  if (!fynecef_cef.loaded) {
    return;
  }
  fynecef_cef.shutdown();
}

void fynecef_cef_do_message_loop_work(void) {
  if (!fynecef_cef.loaded) {
    return;
  }
  fynecef_cef.do_message_loop_work();
}

cef_browser_t* fynecef_cef_browser_host_create_browser_sync(
    const cef_window_info_t* windowInfo,
    struct _cef_client_t* client,
    const cef_string_t* url,
    const struct _cef_browser_settings_t* settings,
    struct _cef_dictionary_value_t* extra_info,
    struct _cef_request_context_t* request_context) {
  if (!fynecef_cef.loaded) {
    fynecef_set_cef_error("CEF library is not loaded");
    return NULL;
  }
  return fynecef_cef.browser_host_create_browser_sync(
      windowInfo, client, url, settings, extra_info, request_context);
}

int fynecef_cef_string_utf8_to_utf16(const char* src,
                                     size_t src_len,
                                     cef_string_utf16_t* output) {
  if (!fynecef_cef.loaded) {
    fynecef_set_cef_error("CEF library is not loaded");
    return 0;
  }
  return fynecef_cef.string_utf8_to_utf16(src, src_len, output);
}

int fynecef_cef_string_utf16_to_utf8(const char16_t* src,
                                     size_t src_len,
                                     cef_string_utf8_t* output) {
  if (!fynecef_cef.loaded) {
    fynecef_set_cef_error("CEF library is not loaded");
    return 0;
  }
  return fynecef_cef.string_utf16_to_utf8(src, src_len, output);
}

void fynecef_cef_string_utf16_clear(cef_string_utf16_t* str) {
  if (!fynecef_cef.loaded) {
    return;
  }
  fynecef_cef.string_utf16_clear(str);
}

void fynecef_cef_string_utf8_clear(cef_string_utf8_t* str) {
  if (!fynecef_cef.loaded) {
    return;
  }
  fynecef_cef.string_utf8_clear(str);
}

void fynecef_cef_string_userfree_utf16_free(cef_string_userfree_utf16_t str) {
  if (!fynecef_cef.loaded) {
    return;
  }
  fynecef_cef.string_userfree_utf16_free(str);
}

#if defined(OS_LINUX) || defined(__linux__)
XDisplay* fynecef_cef_get_xdisplay(void) {
  if (!fynecef_cef.loaded) {
    fynecef_set_cef_error("CEF library is not loaded");
    return NULL;
  }
  return fynecef_cef.get_xdisplay();
}
#endif
