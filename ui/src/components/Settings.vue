	<template>
  <div class="hero-body is-fullheight" v-show="settingsActive">
   <div class="container">
	    <div>
			<p class="has-text-centered is-size-4 has-text-success">Settings</p>
		  <br/>
	 <div class="columns">	

		<div class="column is-two-fifths">
			<label class="label">Select default filtered datasets</label>
			
			<multiselect :multiple="true" v-model="app_conf.global_filter_datasets_new"  track-by="id" label="name" placeholder="Select a dataset" :options="options" :searchable="true">
					<template slot="singleLabel" slot-scope="{ option }"><strong>{{ option.name }}</strong></template>
			</multiselect>
		</div>

		<div class="column">
			<label class="label">Results per page</label>
				<div class="field">
					<div class="select">
						<select v-model="app_conf.page_size_new">
							<option value="9">9</option>
							<option value="18">18</option>
							<option value="36">36</option>
							<option value="63">72</option>
							<option value="90">90</option>
							<option value="150">144</option>
							<option value="200">200</option>
						</select>
				</div>
			</div>
		</div>
		
	    <div class="column">
				<label class="label">Box color</label>
				<div class="field">
					<div class="select">
						<select v-model="app_conf.box_color_new">
						  <option v-for="(option,index) in app_conf.colors" v-bind:value="option">
						    {{ index }}
						  </option>
						</select>
				</div>
			</div>
		</div>
		
	    <div class="column">
			<label class="label">Selected Box color</label>
				<div class="field">
					<div class="select">
						<select v-model="app_conf.selected_box_color_new">
						  <option v-for="(option,index) in app_conf.colors" v-bind:value="option">
						    {{ index }}
						  </option>
						</select>
				</div>
			</div>
		</div>

      </div>
     </div>

		  <div class="column is-pulled-right">		
					<button class="button is-warning" @click="apply">Apply</button>
			</div>

      <!-- <p>Changes are valid only for this session and does not saved.</p> -->
			

		</div>
   </div>
</template>

<script>
export default {
	name: 'Settings',
	props: {
		settingsActive: {
			type: Boolean,
			required: true,
		},
		app_conf: {
			type: Object,
			required: true,
		},
		xref_conf: {
			type: Object,
			required: true,
		}
	},
	data() {
		return {
			perPageResult: 12,
      options: []
		};
	},
	methods: {
		apply: function() {
			this.$emit('apply-settings');
		},
	},
	beforeMount() {
	
	    for(let key in this.xref_conf){
         this.options.push({id:parseInt(key),name:this.xref_conf[key].name})
			}

       this.options.sort(function (a, b) {
				       var nameA=a.name.toLowerCase(), nameB=b.name.toLowerCase();
                if (nameA < nameB) return -1;
                if (nameA > nameB) return 1;
                return 0;
            });
	}
};
</script>

<style src="vue-multiselect/dist/vue-multiselect.min.css"></style>

<style scoped>

</style>
