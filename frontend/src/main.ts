import "./app.css";
import { mount } from "svelte";
import PreviewApp from "./App.svelte";
import FunctionalApp from "./FunctionalApp.svelte";

const target=document.getElementById("app");
if(!target)throw new Error("Missing Svelte mount root");

// Server-chosen HTML entry determines auth mode. Query parameters cannot
// select a privileged transport; backend remains the authority.
const Component=document.documentElement.dataset.app==="functional" ? FunctionalApp : PreviewApp;
mount(Component,{target});
