#ifndef FYNECEF_WEBKIT_DARWIN_H
#define FYNECEF_WEBKIT_DARWIN_H

#include <stdint.h>

typedef struct fynecef_browser_s fynecef_browser_t;

fynecef_browser_t* fynecef_browser_create(uintptr_t go_handle,
                                          uintptr_t window_handle,
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
void fynecef_browser_load_url(fynecef_browser_t* browser, const char* url);
void fynecef_browser_reload(fynecef_browser_t* browser);
void fynecef_browser_stop(fynecef_browser_t* browser);
void fynecef_browser_go_back(fynecef_browser_t* browser);
void fynecef_browser_go_forward(fynecef_browser_t* browser);
void fynecef_browser_set_focus(fynecef_browser_t* browser, int focus);
void fynecef_browser_close(fynecef_browser_t* browser);

extern void goCEFOnAddressChange(uintptr_t handle, char* url);
extern void goCEFOnTitleChange(uintptr_t handle, char* title);
extern void goCEFOnLoadProgress(uintptr_t handle, double progress);
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
extern void goCEFOnBeforeClose(uintptr_t handle);

#endif
