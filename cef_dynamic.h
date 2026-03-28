#ifndef FYNECEF_CEF_DYNAMIC_H
#define FYNECEF_CEF_DYNAMIC_H

#include "include/capi/cef_app_capi.h"
#include "include/capi/cef_browser_capi.h"
#include "include/internal/cef_string.h"

int fynecef_cef_load_library(const char* library_path);
const char* fynecef_cef_last_error(void);

const char* fynecef_cef_api_hash(int version, int entry);
int fynecef_cef_execute_process(const cef_main_args_t* args,
                                cef_app_t* application,
                                void* windows_sandbox_info);
int fynecef_cef_initialize(const cef_main_args_t* args,
                           const struct _cef_settings_t* settings,
                           cef_app_t* application,
                           void* windows_sandbox_info);
void fynecef_cef_shutdown(void);
void fynecef_cef_do_message_loop_work(void);
cef_browser_t* fynecef_cef_browser_host_create_browser_sync(
    const cef_window_info_t* windowInfo,
    struct _cef_client_t* client,
    const cef_string_t* url,
    const struct _cef_browser_settings_t* settings,
    struct _cef_dictionary_value_t* extra_info,
    struct _cef_request_context_t* request_context);
int fynecef_cef_string_utf8_to_utf16(const char* src,
                                     size_t src_len,
                                     cef_string_utf16_t* output);
int fynecef_cef_string_utf16_to_utf8(const char16_t* src,
                                     size_t src_len,
                                     cef_string_utf8_t* output);
void fynecef_cef_string_utf16_clear(cef_string_utf16_t* str);
void fynecef_cef_string_utf8_clear(cef_string_utf8_t* str);
void fynecef_cef_string_userfree_utf16_free(cef_string_userfree_utf16_t str);
#if defined(OS_LINUX) || defined(__linux__)
XDisplay* fynecef_cef_get_xdisplay(void);
#endif

#define cef_api_hash fynecef_cef_api_hash
#define cef_execute_process fynecef_cef_execute_process
#define cef_initialize fynecef_cef_initialize
#define cef_shutdown fynecef_cef_shutdown
#define cef_do_message_loop_work fynecef_cef_do_message_loop_work
#define cef_browser_host_create_browser_sync \
  fynecef_cef_browser_host_create_browser_sync
#define cef_string_utf8_to_utf16 fynecef_cef_string_utf8_to_utf16
#define cef_string_utf16_to_utf8 fynecef_cef_string_utf16_to_utf8
#define cef_string_utf16_clear fynecef_cef_string_utf16_clear
#define cef_string_utf8_clear fynecef_cef_string_utf8_clear
#define cef_string_userfree_utf16_free fynecef_cef_string_userfree_utf16_free
#if defined(OS_LINUX) || defined(__linux__)
#define cef_get_xdisplay fynecef_cef_get_xdisplay
#endif

#endif
