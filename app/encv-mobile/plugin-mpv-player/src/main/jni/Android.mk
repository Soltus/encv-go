LOCAL_PATH := $(call my-dir)

# Prebuilt shared libraries from AAR (extracted to jniLibs/)
# Use abspath to avoid NDK's double jni/ prefix resolution issue
PREBUILT_DIR := $(abspath $(LOCAL_PATH)/../jniLibs/arm64-v8a)

include $(CLEAR_VARS)
LOCAL_MODULE := libswresample
LOCAL_SRC_FILES := $(PREBUILT_DIR)/libswresample.so
include $(PREBUILT_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libavutil
LOCAL_SRC_FILES := $(PREBUILT_DIR)/libavutil.so
include $(PREBUILT_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libavcodec
LOCAL_SRC_FILES := $(PREBUILT_DIR)/libavcodec.so
include $(PREBUILT_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libavformat
LOCAL_SRC_FILES := $(PREBUILT_DIR)/libavformat.so
include $(PREBUILT_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libswscale
LOCAL_SRC_FILES := $(PREBUILT_DIR)/libswscale.so
include $(PREBUILT_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libavfilter
LOCAL_SRC_FILES := $(PREBUILT_DIR)/libavfilter.so
include $(PREBUILT_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libavdevice
LOCAL_SRC_FILES := $(PREBUILT_DIR)/libavdevice.so
include $(PREBUILT_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libxml2
LOCAL_SRC_FILES := $(PREBUILT_DIR)/libxml2.so
include $(PREBUILT_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libcxx_shared
LOCAL_SRC_FILES := $(PREBUILT_DIR)/libc++_shared.so
include $(PREBUILT_SHARED_LIBRARY)

include $(CLEAR_VARS)
LOCAL_MODULE := libmpv
LOCAL_SRC_FILES := $(PREBUILT_DIR)/libmpv.so
include $(PREBUILT_SHARED_LIBRARY)

# --- Build libplayer.so (JNI wrapper) from source ---
include $(CLEAR_VARS)
LOCAL_MODULE    := player
LOCAL_CFLAGS    := -Wno-error -Wno-unused-parameter
LOCAL_CPPFLAGS  += -std=c++11
LOCAL_C_INCLUDES := $(LOCAL_PATH)/include $(LOCAL_PATH)
LOCAL_SRC_FILES := \
	main.cpp \
	render.cpp \
	log.cpp \
	jni_utils.cpp \
	property.cpp \
	event.cpp \
	thumbnail.cpp
LOCAL_LDLIBS    := -llog -latomic
LOCAL_SHARED_LIBRARIES := mpv swscale avcodec swresample
include $(BUILD_SHARED_LIBRARY)
