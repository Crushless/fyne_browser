#ifndef FYNECEF_CEF_LINUX_H
#define FYNECEF_CEF_LINUX_H

#include <stdint.h>

#include "include/capi/cef_app_capi.h"
#include "include/capi/cef_browser_capi.h"
#include "include/capi/cef_client_capi.h"
#include "include/capi/cef_context_menu_handler_capi.h"
#include "include/capi/cef_display_handler_capi.h"
#include "include/capi/cef_life_span_handler_capi.h"
#include "include/capi/cef_load_handler_capi.h"
#include "include/capi/cef_render_handler_capi.h"
#include "include/capi/cef_request_handler_capi.h"
#include "include/capi/cef_resource_request_handler_capi.h"
#include "include/internal/cef_string.h"
#include "include/internal/cef_types_linux.h"

typedef struct fynecef_browser_s fynecef_browser_t;
typedef struct fynecef_menu_item_s fynecef_menu_item_t;
typedef struct fynecef_context_menu_s fynecef_context_menu_t;

struct fynecef_menu_item_s {
  int type;
  int command_id;
  int enabled;
  int checked;
  char* label;
  size_t child_count;
  struct fynecef_menu_item_s* children;
};

struct fynecef_context_menu_s {
  int x;
  int y;
  size_t item_count;
  struct fynecef_menu_item_s* items;
  struct _cef_run_context_menu_callback_t* callback;
};

int fynecef_execute_process(int argc, char** argv, const char* cef_library_path);
const char* fynecef_last_error(void);
int fynecef_initialize(int argc,
                       char** argv,
                       const char* cef_library_path,
                       const char* subprocess_path,
                       const char* resources_dir,
                       const char* locales_dir,
                       const char* cache_path);
void fynecef_shutdown(void);
void fynecef_do_message_loop_work(void);

fynecef_browser_t* fynecef_browser_create(uintptr_t go_handle,
                                          uintptr_t parent_window,
                                          int x,
                                          int y,
                                          int width,
                                          int height,
                                          const char* url);
void fynecef_browser_set_bounds(fynecef_browser_t* browser,
                                int x,
                                int y,
                                int width,
                                int height);
void fynecef_browser_set_windowless_frame_rate(fynecef_browser_t* browser,
                                               int frame_rate);
void fynecef_browser_load_url(fynecef_browser_t* browser, const char* url);
void fynecef_browser_reload(fynecef_browser_t* browser);
void fynecef_browser_stop(fynecef_browser_t* browser);
void fynecef_browser_go_back(fynecef_browser_t* browser);
void fynecef_browser_go_forward(fynecef_browser_t* browser);
void fynecef_browser_set_focus(fynecef_browser_t* browser, int focus);
void fynecef_browser_mouse_move(fynecef_browser_t* browser,
                                int x,
                                int y,
                                uint32_t modifiers,
                                int mouse_leave);
void fynecef_browser_mouse_click(fynecef_browser_t* browser,
                                 int x,
                                 int y,
                                 uint32_t modifiers,
                                 int button,
                                 int mouse_up,
                                 int click_count);
void fynecef_browser_mouse_wheel(fynecef_browser_t* browser,
                                 int x,
                                 int y,
                                 uint32_t modifiers,
                                 int delta_x,
                                 int delta_y);
void fynecef_browser_key_event(fynecef_browser_t* browser,
                               int event_type,
                               uint32_t modifiers,
                               int windows_key_code,
                               int native_key_code,
                               uint16_t character,
                               uint16_t unmodified_character);
void fynecef_browser_close(fynecef_browser_t* browser);
void fynecef_copy_bgra_rect_to_rgba(uint8_t* dst,
                                    int dst_stride,
                                    const uint8_t* src,
                                    int src_stride,
                                    int x,
                                    int y,
                                    int width,
                                    int height);
void fynecef_context_menu_continue(fynecef_context_menu_t* menu,
                                   int command_id,
                                   uint32_t event_flags);
void fynecef_context_menu_cancel(fynecef_context_menu_t* menu);

extern void goCEFOnAddressChange(uintptr_t handle, char* url);
extern void goCEFOnTitleChange(uintptr_t handle, char* title);
extern void goCEFOnLoadProgress(uintptr_t handle, double progress);
extern void goCEFOnCursorChange(uintptr_t handle, int cursor_type);
extern void goCEFOnContextMenu(uintptr_t handle, fynecef_context_menu_t* menu);
extern void goCEFOnLoadingStateChange(uintptr_t handle,
                                      int is_loading,
                                      int can_go_back,
                                      int can_go_forward);
extern void goCEFOnLoadError(uintptr_t handle,
                             int code,
                             char* error_text,
                             char* failed_url);
extern int goCEFOnBeforeResourceLoad(uintptr_t handle,
                                     char* url,
                                     char* method,
                                     char* initiator,
                                     int resource_type,
                                     int is_navigation);
extern void goCEFOnFrame(uintptr_t handle,
                         void* buffer,
                         int width,
                         int height,
                         int stride,
                         size_t dirty_rect_count,
                         cef_rect_t* dirty_rects);
extern void goCEFOnBeforeClose(uintptr_t handle);

#endif
