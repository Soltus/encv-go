LIBFFMPEG_SO_MODULES=(
  "core:fftools/ffmpeg.c fftools/ffmpeg_dec.c fftools/ffmpeg_demux.c fftools/ffmpeg_enc.c fftools/ffmpeg_filter.c fftools/ffmpeg_hw.c fftools/ffmpeg_mux.c fftools/ffmpeg_mux_init.c fftools/ffmpeg_opt.c fftools/ffmpeg_sched.c fftools/cmdutils.c fftools/opt_common.c fftools/sync_queue.c fftools/thread_queue.c"
  "graph:fftools/graph/graphprint.c"
  "textformat:fftools/textformat/avtextformat.c fftools/textformat/tf_compact.c fftools/textformat/tf_default.c fftools/textformat/tf_flat.c fftools/textformat/tf_ini.c fftools/textformat/tf_json.c fftools/textformat/tf_mermaid.c fftools/textformat/tf_xml.c fftools/textformat/tw_avio.c fftools/textformat/tw_buffer.c fftools/textformat/tw_stdout.c"
  "resources:fftools/resources/resman.c fftools/resources/graph.css.c fftools/resources/graph.html.c"
)
LIBFFPROBE_SO_MODULES=(
  "core:fftools/ffprobe.c fftools/cmdutils.c fftools/opt_common.c"
  "textformat_shared:fftools/textformat/avtextformat.c fftools/textformat/tf_compact.c fftools/textformat/tf_default.c fftools/textformat/tf_flat.c fftools/textformat/tf_ini.c fftools/textformat/tf_json.c fftools/textformat/tf_mermaid.c fftools/textformat/tf_xml.c fftools/textformat/tw_avio.c fftools/textformat/tw_buffer.c fftools/textformat/tw_stdout.c"
)
