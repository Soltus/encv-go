package com.encvgo.app

import android.os.Bundle
import android.util.Log
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import androidx.fragment.app.Fragment

class MpvPlayerFragment : Fragment() {
    companion object {
        private const val TAG = "MpvPlayerFragment"

        fun newInstance(filePath: String, fileName: String): MpvPlayerFragment {
            return MpvPlayerFragment().apply {
                arguments = Bundle().apply {
                    putString("file_path", filePath)
                    putString("file_name", fileName)
                }
            }
        }
    }

    override fun onCreateView(inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?): View? {
        Log.i(TAG, "[ModeB-Fragment] onCreateView")
        val fileName = arguments?.getString("file_name", "") ?: ""
        return inflater.inflate(android.R.layout.simple_list_item_1, container, false).apply {
            (this as android.widget.TextView).text = "MPV Fragment Placeholder\nfileName: $fileName\n[experimental - ComboLite does not natively support Fragment proxy]"
            setTextColor(0xFFCCCCCC.toInt())
            gravity = android.view.Gravity.CENTER
            textSize = 16f
        }
    }

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        super.onViewCreated(view, savedInstanceState)
        Log.i(TAG, "[ModeB-Fragment] onViewCreated - fragment created but not functional")
        Log.w(TAG, "[ModeB-Fragment] TODO: ComboLite BaseHostActivity is an Activity, not a Fragment.")
        Log.w(TAG, "[ModeB-Fragment] TODO: ProxyManager is designed for Activity proxy, not Fragment.")
        Log.w(TAG, "[ModeB-Fragment] TODO: Need ComboLite to provide BaseHostFragment or manual ProxyManager wrapping.")
    }

    override fun onDestroyView() {
        Log.i(TAG, "[ModeB-Fragment] onDestroyView")
        super.onDestroyView()
    }
}
